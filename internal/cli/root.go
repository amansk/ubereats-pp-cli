// Package cli is the cobra command tree for ubereats-pp-cli.
package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/amansk/ubereats-pp-cli/internal/auth"
	"github.com/amansk/ubereats-pp-cli/internal/exitcode"
	"github.com/amansk/ubereats-pp-cli/internal/output"
	"github.com/amansk/ubereats-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

const version = "0.1.0"

// Options are global flags.
type Options struct {
	JSON    bool
	Agent   bool
	Quiet   bool
	NoColor bool
	NoInput bool
	Yes     bool
	Home    string
}

func (o Options) Mode() output.Mode {
	m := output.Mode{JSON: o.JSON || o.Agent, Agent: o.Agent, Quiet: o.Quiet, NoColor: o.NoColor || o.Agent}
	return m
}

func (o Options) ResolveHome() (string, error) {
	return auth.HomeDir(o.Home)
}

func (o Options) OpenStore() (*store.DB, string, error) {
	home, err := o.ResolveHome()
	if err != nil {
		return nil, "", err
	}
	db, err := store.Open(home)
	if err != nil {
		return nil, home, err
	}
	return db, home, nil
}

// NewRoot builds the command tree.
func NewRoot() *cobra.Command {
	opt := &Options{}
	cmd := &cobra.Command{
		Use:           "ubereats-pp-cli",
		Short:         "Agent-native Uber Eats buyer CLI (read-only)",
		Long:          "Cookie-auth Uber Eats order history → SQLite → JSON. Read-only. Never prints cookies.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if opt.Agent {
				opt.JSON = true
				opt.NoColor = true
				opt.NoInput = true
				opt.Yes = true
			}
		},
	}
	cmd.PersistentFlags().BoolVar(&opt.JSON, "json", false, "Emit machine-readable JSON")
	cmd.PersistentFlags().BoolVar(&opt.Agent, "agent", false, "JSON + compact + no prompts + no color")
	cmd.PersistentFlags().BoolVar(&opt.Quiet, "quiet", false, "Suppress human chatter")
	cmd.PersistentFlags().BoolVar(&opt.NoColor, "no-color", false, "Disable color")
	cmd.PersistentFlags().BoolVar(&opt.NoInput, "no-input", false, "Never read a TTY prompt")
	cmd.PersistentFlags().BoolVar(&opt.Yes, "yes", false, "Assume yes (reserved)")
	cmd.PersistentFlags().StringVar(&opt.Home, "home", "", "Override state dir ($UBERATS_PP_HOME or ~/.config/ubereats-pp-cli)")

	cmd.AddCommand(newAuthCmd(opt))
	cmd.AddCommand(newDoctorCmd(opt))
	cmd.AddCommand(newSyncCmd(opt))
	cmd.AddCommand(newOrdersCmd(opt))
	cmd.AddCommand(newSpendCmd(opt))
	cmd.AddCommand(newTopItemsCmd(opt))
	cmd.AddCommand(newTopRestaurantsCmd(opt))
	cmd.AddCommand(newFindCmd(opt))
	return cmd
}

// Execute runs the root command and maps errors to exit codes.
func Execute() int {
	cmd := NewRoot()
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	if err := cmd.Execute(); err != nil {
		return handleErr(cmd, err)
	}
	return exitcode.OK
}

func handleErr(cmd *cobra.Command, err error) int {
	code := exitcode.Usage
	var ex *exitcode.Error
	if exitcode.As(err, &ex) {
		code = ex.Code
	}
	// Prefer JSON error if the command wanted machine output.
	if cmd != nil {
		if ex != nil && ex.Silent {
			return code
		}
		jsonFlag, _ := cmd.Root().PersistentFlags().GetBool("json")
		agentFlag, _ := cmd.Root().PersistentFlags().GetBool("agent")
		mode := output.Mode{JSON: jsonFlag || agentFlag, Agent: agentFlag}
		_ = mode.EncodeError(cmd.ErrOrStderr(), err)
		return code
	}
	fmt.Fprintln(os.Stderr, err.Error())
	return code
}

func writeOut(cmd *cobra.Command, opt *Options, data any) error {
	return opt.Mode().Encode(cmd.OutOrStdout(), data)
}

func writeHumanTable(cmd *cobra.Command, opt *Options, headers []string, rows [][]string, jsonData any) error {
	if opt.JSON || opt.Agent {
		return writeOut(cmd, opt, jsonData)
	}
	return output.Table(cmd.OutOrStdout(), headers, rows)
}

func parseDate(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	for _, l := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(l, s); err == nil {
			u := t.UTC()
			return &u, nil
		}
	}
	return nil, exitcode.Usagef("invalid date %q (use YYYY-MM-DD or RFC3339)", s)
}

func mustOpenStore(opt *Options) (*store.DB, error) {
	db, _, err := opt.OpenStore()
	return db, err
}

func isTTY(f *os.File) bool {
	if f == nil {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}
