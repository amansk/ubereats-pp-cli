package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/amansk/ubereats-pp-cli/internal/auth"
	"github.com/amansk/ubereats-pp-cli/internal/client"
	"github.com/amansk/ubereats-pp-cli/internal/exitcode"
	"github.com/amansk/ubereats-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

type check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fatal  bool   `json:"fatal,omitempty"`
}

func newDoctorCmd(opt *Options) *cobra.Command {
	var live bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Health check: home, cookies, SQLite (optional live ping)",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := opt.ResolveHome()
			if err != nil {
				return err
			}
			var checks []check

			// home
			if err := auth.EnsureHome(home); err != nil {
				checks = append(checks, check{Name: "home", Status: "fail", Detail: err.Error(), Fatal: true})
			} else {
				checks = append(checks, check{Name: "home", Status: "pass", Detail: home})
			}

			// cookies
			st := auth.LoadStatus(home)
			if !st.Present {
				checks = append(checks, check{Name: "cookies", Status: "fail", Detail: "no session; run auth login --cookie-file", Fatal: true})
			} else {
				detail := fmt.Sprintf("%d cookies (%s)", st.Count, strings.Join(st.Names, ","))
				checks = append(checks, check{Name: "cookies", Status: "pass", Detail: detail})
			}

			// store
			db, err := store.Open(home)
			if err != nil {
				checks = append(checks, check{Name: "store", Status: "fail", Detail: err.Error(), Fatal: true})
			} else {
				n, _ := db.Count()
				_ = db.Close()
				checks = append(checks, check{Name: "store", Status: "pass", Detail: fmt.Sprintf("%s (%d orders)", db.Path, n)})
			}

			// api
			if live {
				if !st.Present {
					checks = append(checks, check{Name: "api", Status: "skip", Detail: "no session"})
				} else {
					jar, err := auth.Load(home)
					if err != nil {
						checks = append(checks, check{Name: "api", Status: "fail", Detail: "session unreadable", Fatal: true})
					} else {
						base := os.Getenv("UBERATS_PP_BASE_URL")
						c := client.New(jar, base)
						ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
						defer cancel()
						raw, err := c.GetUser(ctx)
						if err != nil {
							checks = append(checks, check{Name: "api", Status: "fail", Detail: err.Error(), Fatal: true})
						} else {
							env := client.EnvelopeStatus(raw)
							if env != "" && !strings.EqualFold(env, "success") {
								checks = append(checks, check{Name: "api", Status: "fail", Detail: "envelope " + env, Fatal: true})
							} else {
								checks = append(checks, check{Name: "api", Status: "pass", Detail: "getUserV1 ok"})
							}
						}
					}
				}
			} else {
				checks = append(checks, check{Name: "api", Status: "skip", Detail: "offline (pass --live to ping Uber Eats)"})
			}

			failed := false
			for _, c := range checks {
				if c.Status == "fail" && c.Fatal {
					failed = true
				}
			}

			payload := map[string]any{"checks": checks, "ok": !failed}
			if opt.JSON || opt.Agent {
				if failed {
					// Still print the report; exit non-zero via typed error after encode.
					_ = writeOut(cmd, opt, payload)
					return authFailFromDoctor(checks)
				}
				return writeOut(cmd, opt, payload)
			}
			for _, c := range checks {
				fmt.Fprintf(cmd.OutOrStdout(), "%-8s %-5s %s\n", c.Name, c.Status, c.Detail)
			}
			if failed {
				return authFailFromDoctor(checks)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "doctor: green")
			return nil
		},
	}
	cmd.Flags().BoolVar(&live, "live", false, "Ping Uber Eats (getUserV1)")
	return cmd
}

func authFailFromDoctor(checks []check) error {
	msg := "doctor found failing checks"
	code := exitcode.API
	for _, c := range checks {
		if c.Status != "fail" {
			continue
		}
		msg = c.Name + ": " + c.Detail
		if c.Name == "cookies" {
			code = exitcode.Auth
		}
		break
	}
	return &exitcode.Error{Code: code, Err: fmt.Errorf("%s", msg), Silent: true}
}
