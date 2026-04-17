package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// DialOptions controls how Client connects. Fields follow the resolved
// values from hosts.yaml + ~/.ssh/config lookup done in
// internal/hosts; DialOptions itself does no resolution.
type DialOptions struct {
	Host           string // host or IP (no user@ prefix)
	Port           int    // default 22
	User           string // default "root"
	KeyPath        string // absolute path to private key
	Passphrase     []byte // optional — empty means unencrypted key
	KnownHostsPath string // absolute path; default ~/.ssh/known_hosts
	// AcceptNewHostKey is explicit opt-in for TOFU: a host that is not
	// yet in known_hosts will be accepted (and appended). An EXISTING
	// mismatched host key is always fatal regardless of this flag.
	AcceptNewHostKey bool
	// ConnectTimeout bounds TCP + SSH handshake. Zero means 15s.
	ConnectTimeout time.Duration
}

// Client is the production Executor backed by golang.org/x/crypto/ssh
// and github.com/pkg/sftp. It multiplexes exec sessions and one
// long-lived sftp subsystem over a single TCP connection.
type Client struct {
	conn *ssh.Client
	sftp *sftp.Client
}

// Dial opens a connection using opts. The caller owns the returned
// Client and must call Close.
func Dial(ctx context.Context, opts DialOptions) (*Client, error) {
	if opts.Host == "" {
		return nil, errors.New("ssh.Dial: Host is required")
	}
	if opts.Port == 0 {
		opts.Port = 22
	}
	if opts.User == "" {
		opts.User = "root"
	}
	if opts.ConnectTimeout == 0 {
		opts.ConnectTimeout = 15 * time.Second
	}

	signer, err := loadSigner(opts.KeyPath, opts.Passphrase)
	if err != nil {
		return nil, err
	}

	hostKey, err := hostKeyCallback(opts.KnownHostsPath, opts.AcceptNewHostKey)
	if err != nil {
		return nil, err
	}

	addr := net.JoinHostPort(opts.Host, fmt.Sprintf("%d", opts.Port))
	cfg := &ssh.ClientConfig{
		User:            opts.User,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hostKey,
		Timeout:         opts.ConnectTimeout,
	}
	// If known_hosts already has entries for this host (possibly from
	// a prior connection or ssh-keyscan), restrict the offered host-key
	// algorithms to those types — otherwise servers like Dropbear that
	// advertise several keys can cause the client to negotiate (say)
	// ssh-rsa while known_hosts only holds an ed25519 line and produce
	// a spurious "host key mismatch".
	if algos := knownHostKeyAlgos(opts.KnownHostsPath, opts.Host, opts.Port); len(algos) > 0 {
		cfg.HostKeyAlgorithms = algos
	}

	d := net.Dialer{Timeout: opts.ConnectTimeout}
	tcp, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("ssh: dial %s: %w", addr, err)
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(tcp, addr, cfg)
	if err != nil {
		_ = tcp.Close()
		return nil, fmt.Errorf("ssh: handshake %s: %w", addr, err)
	}
	conn := ssh.NewClient(sshConn, chans, reqs)

	ftp, err := sftp.NewClient(conn)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ssh: sftp subsystem: %w", err)
	}
	return &Client{conn: conn, sftp: ftp}, nil
}

// Run implements Executor.
func (c *Client) Run(ctx context.Context, command string) ([]byte, []byte, error) {
	sess, err := c.conn.NewSession()
	if err != nil {
		return nil, nil, fmt.Errorf("ssh: new session: %w", err)
	}
	defer sess.Close()

	var out, errBuf bytes.Buffer
	sess.Stdout = &out
	sess.Stderr = &errBuf

	done := make(chan error, 1)
	go func() { done <- sess.Run(command) }()

	select {
	case err := <-done:
		return out.Bytes(), errBuf.Bytes(), err
	case <-ctx.Done():
		_ = sess.Signal(ssh.SIGKILL)
		_ = sess.Close()
		return out.Bytes(), errBuf.Bytes(), ctx.Err()
	}
}

// Upload implements Executor with a tmp-then-rename on the remote so a
// half-written file never shadows the target.
func (c *Client) Upload(ctx context.Context, path string, data []byte, mode fs.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := c.sftp.MkdirAll(dir); err != nil {
		return fmt.Errorf("ssh: mkdir %s: %w", dir, err)
	}
	tmp := path + ".wrtbox.tmp"
	f, err := c.sftp.Create(tmp)
	if err != nil {
		return fmt.Errorf("ssh: create %s: %w", tmp, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = c.sftp.Remove(tmp)
		return fmt.Errorf("ssh: write %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		_ = c.sftp.Remove(tmp)
		return fmt.Errorf("ssh: close %s: %w", tmp, err)
	}
	if err := c.sftp.Chmod(tmp, mode); err != nil {
		_ = c.sftp.Remove(tmp)
		return fmt.Errorf("ssh: chmod %s: %w", tmp, err)
	}
	if err := c.sftp.PosixRename(tmp, path); err != nil {
		// Fallback for servers without posix-rename@openssh.com
		_ = c.sftp.Remove(path)
		if err2 := c.sftp.Rename(tmp, path); err2 != nil {
			_ = c.sftp.Remove(tmp)
			return fmt.Errorf("ssh: rename %s: %w", tmp, err2)
		}
	}
	return nil
}

// Download implements Executor.
func (c *Client) Download(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := c.sftp.Open(path)
	if err != nil {
		return nil, fmt.Errorf("ssh: open %s: %w", path, err)
	}
	defer f.Close()
	return io.ReadAll(f)
}

// MkdirAll implements Executor.
func (c *Client) MkdirAll(ctx context.Context, path string, mode fs.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.sftp.MkdirAll(path); err != nil {
		return err
	}
	return c.sftp.Chmod(path, mode)
}

// Close implements Executor. Idempotent.
func (c *Client) Close() error {
	var first error
	if c.sftp != nil {
		if err := c.sftp.Close(); err != nil {
			first = err
		}
		c.sftp = nil
	}
	if c.conn != nil {
		if err := c.conn.Close(); err != nil && first == nil {
			first = err
		}
		c.conn = nil
	}
	return first
}

func loadSigner(path string, passphrase []byte) (ssh.Signer, error) {
	if path == "" {
		return nil, errors.New("ssh: KeyPath is required (no agent fallback in v1)")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ssh: read key %s: %w", path, err)
	}
	if len(passphrase) == 0 {
		signer, err := ssh.ParsePrivateKey(raw)
		if err == nil {
			return signer, nil
		}
		if !strings.Contains(err.Error(), "passphrase") {
			return nil, fmt.Errorf("ssh: parse key %s: %w", path, err)
		}
		return nil, fmt.Errorf("ssh: key %s is encrypted — set WRTBOX_SSH_PASSPHRASE or use an unencrypted key", path)
	}
	signer, err := ssh.ParsePrivateKeyWithPassphrase(raw, passphrase)
	if err != nil {
		return nil, fmt.Errorf("ssh: decrypt key %s: %w", path, err)
	}
	return signer, nil
}

// knownHostKeyAlgos returns host-key algorithm names recorded for
// host[:port] in path. Returns nil on any problem — the caller falls
// back to the default algorithm list in that case.
func knownHostKeyAlgos(path, host string, port int) []string {
	if path == "" || host == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	// Entries can be "host", "host,host2", "[host]:port", "[host]:port,...".
	norm := knownhosts.Normalize(net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	if port == 22 {
		// Normalize keeps default-port hosts as bare "host".
		norm = host
	}
	seen := map[string]struct{}{}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		hosts := fields[0]
		if strings.HasPrefix(hosts, "@") {
			continue // markers (@cert-authority, @revoked) — skip
		}
		matched := false
		for _, h := range strings.Split(hosts, ",") {
			if h == norm || h == host {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		algo := fields[1]
		if _, ok := seen[algo]; ok {
			continue
		}
		seen[algo] = struct{}{}
		out = append(out, algo)
	}
	return out
}

func hostKeyCallback(path string, acceptNew bool) (ssh.HostKeyCallback, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("ssh: user home: %w", err)
		}
		path = filepath.Join(home, ".ssh", "known_hosts")
	}
	// Ensure the file exists so knownhosts.New doesn't explode on
	// first run.
	if _, err := os.Stat(path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("ssh: stat known_hosts: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("ssh: mkdir known_hosts dir: %w", err)
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, fmt.Errorf("ssh: create known_hosts: %w", err)
		}
		_ = f.Close()
	}
	base, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("ssh: load known_hosts: %w", err)
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		if err := base(hostname, remote, key); err == nil {
			return nil
		} else {
			var ke *knownhosts.KeyError
			if errors.As(err, &ke) && len(ke.Want) == 0 {
				// Unknown host — TOFU path.
				if !acceptNew {
					return fmt.Errorf("ssh: host %s is not in %s; re-run with --accept-new-host-key to trust on first use", hostname, path)
				}
				line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
				f, ferr := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
				if ferr != nil {
					return fmt.Errorf("ssh: append known_hosts: %w", ferr)
				}
				defer f.Close()
				if _, werr := fmt.Fprintln(f, line); werr != nil {
					return fmt.Errorf("ssh: write known_hosts: %w", werr)
				}
				return nil
			}
			return fmt.Errorf("ssh: host key mismatch for %s: %w", hostname, err)
		}
	}, nil
}
