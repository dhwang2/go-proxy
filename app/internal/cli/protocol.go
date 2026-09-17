package cli

import (
	"context"

	"github.com/spf13/cobra"
	"go-proxy/internal/application"
	"go-proxy/internal/crypto"
	"go-proxy/internal/protocol"
)

func registerProtocol(r *Runner, root *cobra.Command) {
	cmd := r.leaf("protocol", "List installed protocol nodes", cobra.NoArgs, func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
		return r.App.ProtocolList(ctx)
	})
	var p application.ProtocolOptions
	var reality bool
	install := r.leaf("install <ss|vless|tuic|anytls|snell>", "Install a protocol or enroll a user", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (application.Result, error) {
		p.Type = protocol.Type(args[0])
		if args[0] == "ss" {
			p.Type = protocol.Shadowsocks
		}
		if args[0] == "vless" && reality {
			p.Type = protocol.VLESSReality
		}
		if args[0] != "ss" && args[0] != "vless" && args[0] != "tuic" && args[0] != "anytls" && args[0] != "snell" {
			return application.Result{}, application.Invalid("unsupported protocol type")
		}
		for flag, allowed := range map[string]bool{"reality": args[0] == "vless", "sni": p.Type == protocol.VLESSReality, "method": args[0] == "ss", "congestion": args[0] == "tuic", "ipv6": args[0] == "snell", "domain": p.Type == protocol.VLESS || p.Type == protocol.TUIC || p.Type == protocol.AnyTLS, "email": p.Type == protocol.VLESS || p.Type == protocol.TUIC || p.Type == protocol.AnyTLS, "shadow-tls": args[0] == "ss" || args[0] == "snell", "shadow-tls-port": p.ShadowTLS, "shadow-tls-sni": p.ShadowTLS} {
			if cmd.Flags().Changed(flag) && !allowed {
				return application.Result{}, application.Invalid("--" + flag + " does not apply to this protocol")
			}
		}
		return r.App.ProtocolInstall(ctx, p)
	})
	f := install.Flags()
	f.StringVar(&p.User, "user", "", "User name (required)")
	f.StringVar(&p.Port, "port", "", "Listen port or auto (required)")
	f.StringVar(&p.Domain, "domain", "", "Certificate domain for TLS protocols")
	f.StringVar(&p.Email, "email", "", "Certificate account email")
	f.BoolVar(&reality, "reality", false, "Use Reality instead of certificate TLS")
	f.StringVar(&p.SNI, "sni", "", "Reality handshake domain (default: random verified candidate)")
	f.StringVar(&p.Method, "method", crypto.DefaultSSMethod, "SS2022 AES-128 or AES-256 method")
	f.StringVar(&p.Congestion, "congestion", "bbr", "TUIC congestion: bbr or cubic")
	f.BoolVar(&p.IPv6, "ipv6", false, "Enable Snell IPv6 egress")
	f.BoolVar(&p.ShadowTLS, "shadow-tls", false, "Wrap SS or Snell with ShadowTLS v3")
	f.StringVar(&p.ShadowTLSPort, "shadow-tls-port", "", "ShadowTLS listen port or auto")
	f.StringVar(&p.ShadowTLSSNI, "shadow-tls-sni", "", "ShadowTLS domain (default: random verified candidate)")
	var removeUser string
	remove := r.leaf("remove <tag>", "Remove a node or one membership", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (application.Result, error) {
		if cmd.Flags().Changed("user") && removeUser == "" {
			return application.Result{}, application.Invalid("--user cannot be empty")
		}
		if err := r.confirm(); err != nil {
			return application.Result{}, err
		}
		return r.App.ProtocolRemove(ctx, args[0], removeUser)
	})
	remove.Flags().StringVar(&removeUser, "user", "", "Remove only this membership")
	cmd.AddCommand(install, remove)
	root.AddCommand(cmd)
}
