package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"bu1ld/internal/lsp"

	"github.com/spf13/cobra"
)

type options struct {
	in  io.Reader
	out io.Writer
	err io.Writer
}

func Execute() error {
	cmd := NewRootCommand(os.Stdin, os.Stdout, os.Stderr)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), err)
		return err
	}
	return nil
}

func NewRootCommand(in io.Reader, out io.Writer, errOut io.Writer) *cobra.Command {
	opts := options{
		in:  in,
		out: out,
		err: errOut,
	}

	cmd := &cobra.Command{
		Use:           "lsp",
		Short:         "bu1ld DSL language server",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStdio(cmd.Context(), &opts)
		},
	}
	cmd.SetOut(errOut)
	cmd.SetErr(errOut)
	cmd.AddCommand(newStdioCommand(&opts))
	return cmd
}

func newStdioCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:           "stdio",
		Short:         "Serve the bu1ld DSL LSP over stdio",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStdio(cmd.Context(), opts)
		},
	}
}

func runStdio(ctx context.Context, opts *options) error {
	return lsp.Run(ctx, opts.in, opts.out)
}
