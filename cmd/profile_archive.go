package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"mmcli/internal/config"
	"mmcli/internal/profile"
)

func init() {
	profileCmd.AddCommand(&cobra.Command{
		Use:   "export <name> <archive.zip>",
		Short: "Export installed mods and settings to a portable archive",
		Long:  "Export exact installed files, versions, sources and disabled states. Includes mod configs and local mods; excludes the BepInEx runtime, server connections, and local modpack paths. Does not overwrite existing archives.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			reg, err := config.LoadRegistry(paths)
			if err != nil {
				return err
			}
			config.MigrateProfileSettings(&cfg, &reg, cfg.ActiveProfile)
			if err := profile.ExportArchive(paths, reg, args[0], args[1]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Exported profile %q to %s\n", args[0], args[1])
			return nil
		},
	})
	profileCmd.AddCommand(&cobra.Command{
		Use:   "restore <name> <archive.zip>",
		Short: "Restore a portable archive into a new profile",
		Long:  "Restore an mmcli export without downloading or updating mods. Run mmcli init first. Existing profiles are never overwritten; the active profile is unchanged. Reconnect remote servers and local modpack paths separately.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, _, err := loadConfig()
			if err != nil {
				return err
			}
			reg, err := config.LoadRegistry(paths)
			if err != nil {
				return err
			}
			if err := profile.ImportArchive(paths, &reg, args[0], args[1]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Restored profile %q. Activate with: mmcli profile switch %s\n", args[0], args[0])
			return nil
		},
	})
}
