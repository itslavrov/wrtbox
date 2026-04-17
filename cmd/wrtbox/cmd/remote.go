package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/itslavrov/wrtbox/internal/hosts"
	"github.com/itslavrov/wrtbox/internal/ssh"
)

// remoteFlags are the SSH-related flags shared by apply/diff/rollback.
type remoteFlags struct {
	router    string
	host      string
	key       string
	acceptNew bool
}

func (r *remoteFlags) bind(c *cobra.Command) {
	c.Flags().StringVar(&r.router, "router", "", "router alias from ~/.config/wrtbox/hosts.yaml")
	c.Flags().StringVar(&r.host, "host", "", "SSH target (user@host[:port]) — overrides --router")
	c.Flags().StringVar(&r.key, "key", "", "SSH private key path (default: ~/.ssh/id_ed25519 etc.)")
	c.Flags().BoolVar(&r.acceptNew, "accept-new-host-key", false, "trust unknown hosts on first connect (TOFU)")
}

// dial resolves the target and opens an SSH connection. Caller owns
// the returned client and must Close it.
func (r *remoteFlags) dial(ctx context.Context) (*ssh.Client, hosts.Router, error) {
	if r.router == "" && r.host == "" {
		return nil, hosts.Router{}, fmt.Errorf("either --router or --host is required")
	}
	path, err := hosts.DefaultPath()
	if err != nil {
		return nil, hosts.Router{}, err
	}
	reg, err := hosts.Load(path)
	if err != nil {
		return nil, hosts.Router{}, err
	}
	name := r.router
	if name == "" {
		name = r.host
	}
	rt, err := reg.Resolve(name)
	if err != nil {
		return nil, hosts.Router{}, err
	}
	rt, err = reg.Override(rt, r.host, r.key)
	if err != nil {
		return nil, hosts.Router{}, err
	}
	if rt.Key == "" {
		return nil, hosts.Router{}, fmt.Errorf("no SSH key found for %s (pass --key or configure ~/.ssh/config IdentityFile)", name)
	}

	passphrase := []byte(os.Getenv("WRTBOX_SSH_PASSPHRASE"))
	cli, err := ssh.Dial(ctx, ssh.DialOptions{
		Host:             rt.Host,
		Port:             rt.Port,
		User:             rt.User,
		KeyPath:          rt.Key,
		Passphrase:       passphrase,
		KnownHostsPath:   rt.KnownHosts,
		AcceptNewHostKey: r.acceptNew,
	})
	if err != nil {
		return nil, rt, err
	}
	return cli, rt, nil
}
