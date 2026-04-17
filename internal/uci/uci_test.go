package uci_test

import (
	"bytes"
	"testing"

	"github.com/itslavrov/wrtbox/internal/uci"
)

func TestRender(t *testing.T) {
	p := uci.Package{
		Name: "network",
		Sections: []uci.Section{
			{
				Type: "interface", Name: "lan",
				Items: []uci.Item{
					uci.Opt("device", "br-lan"),
					uci.Opt("proto", "static"),
					uci.Lst("ipaddr", "192.168.1.1/24"),
				},
			},
		},
	}
	var buf bytes.Buffer
	if err := uci.Render(&buf, p); err != nil {
		t.Fatalf("render: %v", err)
	}
	want := "package network\n\nconfig interface 'lan'\n\toption device 'br-lan'\n\toption proto 'static'\n\tlist ipaddr '192.168.1.1/24'\n"
	if got := buf.String(); got != want {
		t.Fatalf("unexpected output:\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

func TestRenderEscapesSingleQuote(t *testing.T) {
	p := uci.Package{Name: "system", Sections: []uci.Section{
		{Type: "system", Items: []uci.Item{uci.Opt("description", "it's mine")}},
	}}
	var buf bytes.Buffer
	_ = uci.Render(&buf, p)
	want := "package system\n\nconfig system\n\toption description 'it'\\''s mine'\n"
	if got := buf.String(); got != want {
		t.Fatalf("unexpected output:\n%s", got)
	}
}
