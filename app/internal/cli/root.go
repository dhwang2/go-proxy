package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"go-proxy/internal/application"
)

type Runner struct {
	App               *application.App
	In                io.Reader
	Out, Err          io.Writer
	Version, Revision string
	JSON, Yes         bool
	Timeout           time.Duration
	mu                sync.Mutex
	executed          bool
	result            application.Result
	wroteResult       bool
	ioCtx             context.Context
}

func New(version, revision string, in io.Reader, out, stderr io.Writer) *Runner {
	r := &Runner{In: in, Out: out, Err: stderr, Version: version, Revision: revision, ioCtx: context.Background()}
	r.App = application.New(r.progress)
	return r
}

func (r *Runner) progress(message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintln(contextWriter{ctx: r.ioCtx, writer: r.Err}, redact(message))
}

func (r *Runner) confirm() error {
	if !r.Yes {
		return application.Invalid("this operation requires --yes")
	}
	return nil
}

func (r *Runner) leaf(use, short string, args cobra.PositionalArgs, fn func(context.Context, *cobra.Command, []string) (application.Result, error)) *cobra.Command {
	cmd := &cobra.Command{Use: use, Short: short, Args: args}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		r.executed = true
		stream, _ := cmd.Flags().GetBool("follow")
		stream = stream || cmd.Name() == "watchdog"
		if stream && r.JSON {
			return application.Invalid("--json is not available for streaming commands")
		}
		if r.Timeout < 0 || cmd.Flags().Changed("timeout") && r.Timeout == 0 {
			return application.Invalid("--timeout must be positive")
		}
		ctx := cmd.Context()
		cancel := func() {}
		if !stream || r.Timeout > 0 {
			timeout := r.Timeout
			if timeout == 0 {
				timeout = 2500 * time.Millisecond
				if mutation(cmd) {
					timeout = 2 * time.Minute
				}
				if cmd.Name() == "install" || cmd.Name() == "ensure" || cmd.Name() == "update" && mutation(cmd) {
					timeout = 5 * time.Minute
				}
			}
			ctx, cancel = context.WithTimeout(ctx, timeout)
		}
		defer cancel()
		r.mu.Lock()
		r.ioCtx = ctx
		r.mu.Unlock()
		if mutation(cmd) {
			r.progress("starting " + cmd.CommandPath())
			done := make(chan struct{})
			defer close(done)
			go func() {
				ticker := time.NewTicker(5 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-ticker.C:
						r.progress("operation in progress")
					case <-done:
						return
					case <-ctx.Done():
						return
					}
				}
			}()
		}
		result, err := fn(ctx, cmd, args)
		r.result = result
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if result.Silent {
			return nil
		}
		if result.Raw != nil {
			r.wroteResult = true
			_, err = (contextWriter{ctx: ctx, writer: r.Out}).Write(result.Raw)
			return err
		}
		r.wroteResult = true
		if r.JSON {
			if data, ok := result.Data.(json.RawMessage); ok {
				return writeEnvelope(contextWriter{ctx: ctx, writer: r.Out}, result.Changed, data)
			}
			return json.NewEncoder(contextWriter{ctx: ctx, writer: r.Out}).Encode(map[string]any{"ok": true, "changed": result.Changed, "data": result.Data})
		}
		encoder := json.NewEncoder(contextWriter{ctx: ctx, writer: r.Out})
		encoder.SetIndent("", "  ")
		return encoder.Encode(result.Data)
	}
	return cmd
}

// writeEnvelope streams an already encoded payload in the envelope encoding/json produces for the
// result map. Re-encoding it would copy and revalidate the whole stream, which a capacity-scale
// export cannot afford.
func writeEnvelope(w io.Writer, changed bool, data json.RawMessage) error {
	header := `{"changed":false,"data":`
	if changed {
		header = `{"changed":true,"data":`
	}
	for _, chunk := range [][]byte{[]byte(header), data, []byte(`,"ok":true}` + "\n")} {
		if _, err := w.Write(chunk); err != nil {
			return err
		}
	}
	return nil
}

func mutation(cmd *cobra.Command) bool {
	if check, _ := cmd.Flags().GetBool("check"); check {
		return false
	}
	if preview, _ := cmd.Flags().GetBool("preview"); preview {
		return false
	}
	if cmd.Name() == "direct" {
		return cmd.Flags().Changed("strategy")
	}
	switch cmd.Name() {
	case "init", "install", "add", "remove", "delete", "rename", "set", "modify", "clear", "apply", "enable", "disable", "ensure", "update", "uninstall", "start", "stop", "restart", "sync-dns":
		return true
	}
	return false
}

func (r *Runner) Root() *cobra.Command {
	root := &cobra.Command{Use: "gproxy", Short: "go-proxy: non-interactive proxy management", SilenceUsage: true, SilenceErrors: true, Args: cobra.NoArgs}
	root.SetIn(r.In)
	root.SetOut(r.Out)
	root.SetErr(r.Err)
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentFlags().BoolVar(&r.JSON, "json", false, "write a structured JSON result")
	root.PersistentFlags().BoolVar(&r.Yes, "yes", false, "confirm the selected destructive operation")
	root.PersistentFlags().DurationVar(&r.Timeout, "timeout", 0, "override the operation deadline (for example 30s)")
	root.RunE = func(cmd *cobra.Command, args []string) error { return cmd.Help() }
	root.AddCommand(r.leaf("version", "show the build version", cobra.NoArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		if !r.JSON {
			return application.Result{Raw: []byte("go-proxy " + r.Version + "\n")}, nil
		}
		revision := r.Revision
		if revision == "" {
			if info, ok := debug.ReadBuildInfo(); ok {
				for _, setting := range info.Settings {
					if setting.Key == "vcs.revision" {
						revision = setting.Value
					}
				}
			}
		}
		return application.Result{Data: map[string]any{"name": "go-proxy", "command": "gproxy", "version": r.Version, "revision": revision}}, nil
	}))
	registerProtocol(r, root)
	registerUser(r, root)
	registerSub(r, root)
	registerSystem(r, root)
	registerNetwork(r, root)
	registerRouting(r, root)
	registerInspect(r, root)
	var groups func(*cobra.Command)
	groups = func(c *cobra.Command) {
		if !c.Runnable() && c.HasSubCommands() {
			c.Args = cobra.NoArgs
			c.RunE = func(cmd *cobra.Command, args []string) error {
				names := []string{}
				for _, child := range cmd.Commands() {
					if child.IsAvailableCommand() {
						names = append(names, child.Name())
					}
				}
				if len(names) == 0 {
					return application.Invalid(cmd.CommandPath() + " requires a subcommand")
				}
				return application.Invalid(cmd.CommandPath() + " requires a subcommand: " + strings.Join(names, ", "))
			}
		}
		for _, child := range c.Commands() {
			groups(child)
		}
	}
	groups(root)
	return root
}

func (r *Runner) Run(ctx context.Context, args []string) int {
	r.executed = false
	r.result = application.Result{}
	r.wroteResult = false
	root := r.Root()
	helpCtx, helpCancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer helpCancel()
	root.SetOut(contextWriter{ctx: helpCtx, writer: r.Out})
	root.SetErr(contextWriter{ctx: helpCtx, writer: r.Err})
	root.SetArgs(args)
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--json" || arg == "--json=true" {
			r.JSON = true
		}
		if arg == "--json=false" {
			r.JSON = false
		}
	}
	err := root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	code := 1
	detail := &application.Error{Code: "operation_failed", Message: redact(err.Error())}
	var appErr *application.Error
	if errors.As(err, &appErr) {
		copy := *appErr
		detail = &copy
		detail.Message = redact(detail.Message)
	}
	if !r.executed {
		detail.Code = "invalid_argument"
	}
	if detail.Code == "invalid_argument" {
		code = 2
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		code = 130
		detail.Code = "cancelled"
		detail.Message = "operation cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		detail.Code = "timeout"
		detail.Message = "operation exceeded its deadline"
	}
	var data any
	if detail.Data != nil {
		data = detail.Data
	}
	errorCtx, errorCancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer errorCancel()
	r.mu.Lock()
	r.ioCtx = errorCtx
	r.mu.Unlock()
	if r.JSON && !r.wroteResult {
		_ = json.NewEncoder(contextWriter{ctx: errorCtx, writer: r.Out}).Encode(map[string]any{"ok": false, "changed": r.result.Changed || detail.Changed, "data": data, "error": detail})
	} else {
		r.progress("error: " + detail.Message)
	}
	return code
}

var secretField = regexp.MustCompile(`(?i)(psk|password|private_key|private-key|shadow-tls-password)(["']?\s*[:=]\s*["']?)([^,\s"}]+)`)
var urlUser = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^/@\s]+@`)

func redact(s string) string {
	s = secretField.ReplaceAllString(s, "${1}${2}<redacted>")
	s = urlUser.ReplaceAllString(s, "${1}<redacted>@")
	return strings.TrimSpace(s)
}
