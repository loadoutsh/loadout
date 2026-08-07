package main

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/loadoutsh/loadout/internal/catalog"
	"github.com/loadoutsh/loadout/internal/server"
)

func newUICmd() *cobra.Command {
	var (
		port    int
		noOpen  bool
		printer bool
	)

	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Open the local web interface",
		Long: `Start a small web server on 127.0.0.1 and open it in your browser.

It runs only while this command runs; there is no daemon. Because the
interface can rewrite the shell file your shell sources, every request must
carry a token generated for this run, and requests from any other origin are
refused. The token is in the URL, so treat that URL as a secret.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cat, err := catalog.Load()
			if err != nil {
				return err
			}
			binDir, err := managedBinDir()
			if err != nil {
				return err
			}
			srv, err := server.New(cat, binDir)
			if err != nil {
				return err
			}

			// Ctrl-C should stop the server, not kill it mid-write.
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			return srv.Serve(ctx, port, func(url string) {
				if printer {
					cmd.Println(url)
					return
				}
				cmd.Printf("loadout ui is running at\n\n  %s\n\n", url)
				cmd.Println("That URL contains a one-time token for this run. Ctrl-C to stop.")
				if !noOpen {
					if err := openBrowser(url); err != nil {
						cmd.PrintErrf("could not open a browser (%v); open the URL above yourself\n", err)
					}
				}
			})
		},
	}

	cmd.Flags().IntVar(&port, "port", 0, "port to listen on (0 picks a free one)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "do not launch a browser")
	cmd.Flags().BoolVar(&printer, "print-url", false, "print only the URL, for scripting")
	return cmd
}

// openBrowser launches the platform's default handler.
func openBrowser(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	default:
		cmd = "xdg-open"
	}
	args = append(args, url)
	c := exec.Command(cmd, args...)
	// Detach: the browser must outlive nothing here, and its output should
	// not interleave with ours.
	c.Stdout, c.Stderr = nil, nil
	return c.Start()
}
