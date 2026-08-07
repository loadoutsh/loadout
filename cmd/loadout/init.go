package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/wesscoby/loadout/internal/catalog"
	"github.com/wesscoby/loadout/internal/config"
)

func newInitCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Write a starter config",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.Path()
			if err != nil {
				return err
			}
			if _, err := os.Stat(path); err == nil && !force {
				return fmt.Errorf("%s already exists (use --force to overwrite)", path)
			}
			if err := config.Default().Save(); err != nil {
				return err
			}
			cmd.Printf("wrote %s\n", path)
			cmd.Println("edit it, then run 'loadout apply'")
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing config")
	return cmd
}

func newCatalogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Show what the shipped catalog contains",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cat, err := catalog.Load()
			if err != nil {
				return err
			}
			core, packed := 0, map[string]int{}
			for _, a := range cat.Aliases {
				if a.Pack == "" {
					core++
					continue
				}
				packed[a.Pack]++
			}
			cmd.Printf("aliases:   %d (%d core, %d in packs)\n", len(cat.Aliases), core, len(cat.Aliases)-core)
			cmd.Printf("functions: %d\n", len(cat.Functions))
			cmd.Println("packs:")
			for _, p := range cat.Packs() {
				cmd.Printf("  %-14s %d aliases\n", p, packed[p])
			}
			return nil
		},
	}
	return cmd
}
