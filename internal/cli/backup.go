package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/server/dto"
)

func newBackupsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backups",
		Short: "Create, list, restore, and verify volume backups",
	}
	cmd.AddCommand(
		newBackupsListCmd(),
		newBackupsShowCmd(),
		newBackupsCreateCmd(),
		newBackupsRemoveCmd(),
		newBackupsVerifyCmd(),
		newBackupsRestoreCmd(),
	)
	return cmd
}

func newBackupsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List backups for the current project",
		Example: `ore backups list
ore backups list --volume playground_lobby_world`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			volume, _ := cmd.Flags().GetString("volume")

			remote, err := connectNode()
			if err != nil {
				return err
			}
			defer func() { _ = remote.Close() }()

			bs, err := remote.Backups(cmd.Context(), volume)
			if err != nil {
				return err
			}
			return writeBackupTable(bs)
		},
	}
	cmd.Flags().String("volume", "", "filter by docker volume name")
	return cmd
}

func newBackupsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "show <id>",
		Short:   "Show backup details",
		Example: `ore backups show 01HVZR9PJQ4Y0EKV6GS7CV5FXW`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			remote, err := connectNode()
			if err != nil {
				return err
			}
			defer func() { _ = remote.Close() }()

			b, err := remote.Backup(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return writeBackupDetail(b)
		},
	}
}

func newBackupsCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <volume>",
		Short: "Snapshot a volume into a backup",
		Example: `ore backups create playground_lobby_world
ore backups create playground_lobby_world --tag weekly`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tags, _ := cmd.Flags().GetStringSlice("tag")

			remote, err := connectNode()
			if err != nil {
				return err
			}
			defer func() { _ = remote.Close() }()

			return remote.BackupCreate(cmd.Context(), args[0], tags)
		},
	}
	cmd.Flags().StringSlice("tag", nil, "tag(s) to attach to the backup")
	return cmd
}

func newBackupsRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "remove <id>",
		Short:   "Remove a backup",
		Example: `ore backups remove 01HVZR9PJQ4Y0EKV6GS7CV5FXW`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			remote, err := connectNode()
			if err != nil {
				return err
			}
			defer func() { _ = remote.Close() }()

			if err := remote.BackupRemove(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Printf("removed backup %q\n", args[0])
			return nil
		},
	}
}

func newBackupsVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "verify <id>",
		Short:   "Verify a backup's integrity via sha256",
		Example: `ore backups verify 01HVZR9PJQ4Y0EKV6GS7CV5FXW`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			remote, err := connectNode()
			if err != nil {
				return err
			}
			defer func() { _ = remote.Close() }()

			return remote.BackupVerify(cmd.Context(), args[0])
		},
	}
}

func newBackupsRestoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore <id>",
		Short: "Restore a backup into its source volume",
		Example: `ore backups restore 01HVZR9PJQ4Y0EKV6GS7CV5FXW
ore backups restore 01HVZR9PJQ4Y0EKV6GS7CV5FXW --keep-pre-restore`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			keep, _ := cmd.Flags().GetBool("keep-pre-restore")

			remote, err := connectNode()
			if err != nil {
				return err
			}
			defer func() { _ = remote.Close() }()

			return remote.BackupRestore(cmd.Context(), args[0], keep)
		},
	}
	cmd.Flags().Bool("keep-pre-restore", false, "take a pre-restore safety backup before extracting")
	return cmd
}

func writeBackupTable(bs []dto.BackupResponse) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tVOLUME\tSTATUS\tSIZE\tCOMPRESSED\tCREATED")
	for _, b := range bs {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			b.ID,
			fallback(b.Volume, "—"),
			b.Status,
			formatSize(b.SizeBytes),
			formatSize(b.Compressed),
			formatCreated(b.CreatedAt),
		)
	}
	return w.Flush()
}

func writeBackupDetail(b *dto.BackupResponse) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fields := [][2]string{
		{"ID", b.ID},
		{"Project", b.Project},
		{"Volume", b.Volume},
		{"Logical", b.LogicalName},
		{"Kind", b.Kind},
		{"Status", b.Status},
		{"Created", formatCreated(b.CreatedAt)},
		{"Finished", formatCreated(b.FinishedAt)},
		{"Size", formatSize(b.SizeBytes)},
		{"Compressed", formatSize(b.Compressed)},
		{"Algorithm", b.Algorithm},
		{"Checksum", b.Checksum},
		{"Verified", formatCreated(b.Verified)},
		{"Error", b.Error},
	}
	for _, f := range fields {
		_, _ = fmt.Fprintf(w, "%s:\t%s\n", f[0], fallback(f[1], "—"))
	}
	if len(b.Storage) > 0 {
		_, _ = fmt.Fprintln(w, "Storage:")
		for _, s := range b.Storage {
			_, _ = fmt.Fprintf(w, "  %s\t%s\n", s.Backend, s.URI)
		}
	}
	if len(b.Tags) > 0 {
		_, _ = fmt.Fprintf(w, "Tags:\t%s\n", strings.Join(b.Tags, ", "))
	}
	return w.Flush()
}
