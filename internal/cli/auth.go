package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/amansk/ubereats-pp-cli/internal/auth"
	"github.com/amansk/ubereats-pp-cli/internal/exitcode"
	"github.com/spf13/cobra"
)

func newAuthCmd(opt *Options) *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Import and inspect the Uber Eats cookie session"}
	cmd.AddCommand(newAuthLoginCmd(opt))
	cmd.AddCommand(newAuthStatusCmd(opt))
	cmd.AddCommand(newAuthLogoutCmd(opt))
	return cmd
}

func newAuthLoginCmd(opt *Options) *cobra.Command {
	var cookieFile string
	var chrome bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Import session cookies (never printed)",
		Long: `Import a browser session for ubereats.com / auth.uber.com.

Accepts Netscape cookies.txt, a Chrome/Playwright JSON cookie export, or a raw
Cookie header. Use --cookie-file - to read stdin. --chrome is best-effort and
often fails on encrypted Chrome profiles — fall back to a file export.

Cookie values are stored with mode 0600 and are never written to stdout/stderr.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := opt.ResolveHome()
			if err != nil {
				return err
			}
			var st auth.Store
			switch {
			case chrome && cookieFile != "":
				return exitcode.Usagef("use either --chrome or --cookie-file, not both")
			case chrome:
				st, err = auth.ImportChrome()
				if err != nil {
					return err
				}
			case cookieFile != "":
				st, err = readCookieSource(cmd, opt, cookieFile)
				if err != nil {
					return err
				}
			default:
				if opt.NoInput || opt.Agent || !isTTY(os.Stdin) {
					// Non-interactive: treat stdin as cookie payload when piped.
					if !isTTY(os.Stdin) {
						st, err = auth.ParseCookies(os.Stdin, "stdin")
						if err != nil {
							return err
						}
						break
					}
					return exitcode.Usagef("auth login requires --cookie-file or --chrome")
				}
				return exitcode.Usagef("auth login requires --cookie-file PATH or --chrome")
			}
			if err := auth.Save(home, st); err != nil {
				return err
			}
			status := st.Status(auth.CookiePath(home))
			return writeOut(cmd, opt, map[string]any{
				"imported": status.Count,
				"names":    status.Names,
				"source":   status.Source,
				"path":     status.Path,
				"hints":    status.Hints,
			})
		},
	}
	cmd.Flags().StringVar(&cookieFile, "cookie-file", "", "Cookie file path, or - for stdin")
	cmd.Flags().BoolVar(&chrome, "chrome", false, "Best-effort import from local Chrome cookie DB")
	return cmd
}

func readCookieSource(cmd *cobra.Command, opt *Options, path string) (auth.Store, error) {
	if path == "-" {
		if opt.NoInput && isTTY(os.Stdin) {
			return auth.Store{}, exitcode.Usagef("stdin is a TTY; pass a file path or pipe cookies")
		}
		return auth.ParseCookies(io.LimitReader(cmd.InOrStdin(), 1<<20), "stdin")
	}
	f, err := os.Open(path)
	if err != nil {
		return auth.Store{}, exitcode.Authf("open cookie file: %w", err)
	}
	defer f.Close()
	return auth.ParseCookies(io.LimitReader(f, 1<<20), "file")
}

func newAuthStatusCmd(opt *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show redacted session info (names only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := opt.ResolveHome()
			if err != nil {
				return err
			}
			st := auth.LoadStatus(home)
			if !opt.JSON && !opt.Agent {
				if !st.Present {
					fmt.Fprintln(cmd.OutOrStdout(), "session: missing")
					fmt.Fprintf(cmd.OutOrStdout(), "path: %s\n", st.Path)
					return nil
				}
				fmt.Fprintf(cmd.OutOrStdout(), "session: present (%d cookies)\n", st.Count)
				fmt.Fprintf(cmd.OutOrStdout(), "names: %s\n", strings.Join(st.Names, ", "))
				fmt.Fprintf(cmd.OutOrStdout(), "source: %s\n", st.Source)
				if !st.ImportedAt.IsZero() {
					fmt.Fprintf(cmd.OutOrStdout(), "imported_at: %s\n", st.ImportedAt.UTC().Format("2006-01-02T15:04:05Z"))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "path: %s\n", st.Path)
				return nil
			}
			return writeOut(cmd, opt, st)
		},
	}
}

func newAuthLogoutCmd(opt *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Delete the stored cookie session",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := opt.ResolveHome()
			if err != nil {
				return err
			}
			if err := auth.Delete(home); err != nil {
				return err
			}
			return writeOut(cmd, opt, map[string]any{"logged_out": true})
		},
	}
}
