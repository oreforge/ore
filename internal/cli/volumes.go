package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/deploy"
	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/server/dto"
	"github.com/oreforge/ore/internal/spec"
	"github.com/oreforge/ore/internal/volumes"
)

func newVolumesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "volumes",
		Short: "Manage ore-managed Docker volumes",
	}
	cmd.AddCommand(
		newVolumesListCmd(),
		newVolumesShowCmd(),
		newVolumesSizeCmd(),
		newVolumesRemoveCmd(),
		newVolumesPruneCmd(),
	)
	return cmd
}

func newVolumesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List volumes for the current project",
		Example: `ore volumes list`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			vols, err := fetchVolumes(cmd)
			if err != nil {
				return err
			}
			return writeVolumeTable(vols)
		},
	}
}

func newVolumesShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "show <name>",
		Short:   "Show volume details",
		Example: `ore volumes show playground_lobby_world`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := fetchVolume(cmd, args[0])
			if err != nil {
				return err
			}
			return writeVolumeDetail(v)
		},
	}
}

func fetchVolumes(cmd *cobra.Command) ([]dto.VolumeResponse, error) {
	local, specPath, remote, err := resolveMode(cmd)
	if err != nil {
		return nil, err
	}
	if remote != nil {
		defer func() { _ = remote.Close() }()
	}

	if local {
		return localVolumes(cmd.Context(), specPath)
	}
	return remote.Volumes(cmd.Context())
}

func fetchVolume(cmd *cobra.Command, name string) (*dto.VolumeResponse, error) {
	local, specPath, remote, err := resolveMode(cmd)
	if err != nil {
		return nil, err
	}
	if remote != nil {
		defer func() { _ = remote.Close() }()
	}

	if local {
		return localVolume(cmd.Context(), specPath, name)
	}
	return remote.Volume(cmd.Context(), name)
}

func localVolumes(ctx context.Context, specPath string) ([]dto.VolumeResponse, error) {
	s, err := spec.Load(specPath)
	if err != nil {
		return nil, err
	}
	svc, cleanup, err := localVolumeService(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	vs, err := svc.List(ctx, s.Network)
	if err != nil {
		return nil, err
	}
	out := make([]dto.VolumeResponse, len(vs))
	for i, v := range vs {
		out[i] = dto.NewVolumeResponse(v)
	}
	return out, nil
}

func localVolume(ctx context.Context, specPath, name string) (*dto.VolumeResponse, error) {
	s, err := spec.Load(specPath)
	if err != nil {
		return nil, err
	}
	svc, cleanup, err := localVolumeService(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	v, err := svc.Inspect(ctx, name)
	if err != nil {
		if errors.Is(err, volumes.ErrNotFound) {
			return nil, fmt.Errorf("volume %q not found", name)
		}
		return nil, err
	}
	if v.Project != s.Network {
		return nil, fmt.Errorf("volume %q belongs to a different project", name)
	}
	resp := dto.NewVolumeResponse(v)
	return &resp, nil
}

func localVolumeService(ctx context.Context) (*volumes.Service, func(), error) {
	dockerClient, err := docker.New(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to Docker: %w", err)
	}
	return volumes.New(dockerClient, logger), func() { _ = dockerClient.Close() }, nil
}

func writeVolumeTable(vols []dto.VolumeResponse) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tOWNER\tLOGICAL\tSIZE\tIN USE\tCREATED")
	for _, v := range vols {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			v.Name,
			fallback(v.Owner, "—"),
			fallback(v.Logical, "—"),
			formatSize(v.SizeBytes),
			formatInUse(v.InUseBy),
			formatCreated(v.CreatedAt),
		)
	}
	return w.Flush()
}

func writeVolumeDetail(v *dto.VolumeResponse) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fields := []struct{ k, val string }{
		{"Name", v.Name},
		{"Project", v.Project},
		{"Owner", v.Owner},
		{"Owner kind", v.OwnerKind},
		{"Logical", v.Logical},
		{"Driver", v.Driver},
		{"Mountpoint", v.Mountpoint},
		{"Created", formatCreated(v.CreatedAt)},
		{"Size", formatSize(v.SizeBytes)},
		{"In use by", formatInUse(v.InUseBy)},
	}
	for _, f := range fields {
		_, _ = fmt.Fprintf(w, "%s:\t%s\n", f.k, fallback(f.val, "—"))
	}
	return w.Flush()
}

func formatSize(b int64) string {
	if b < 0 {
		return "—"
	}
	const (
		kb = 1 << 10
		mb = 1 << 20
		gb = 1 << 30
		tb = 1 << 40
	)
	switch {
	case b >= tb:
		return strconv.FormatFloat(float64(b)/tb, 'f', 2, 64) + " TiB"
	case b >= gb:
		return strconv.FormatFloat(float64(b)/gb, 'f', 2, 64) + " GiB"
	case b >= mb:
		return strconv.FormatFloat(float64(b)/mb, 'f', 2, 64) + " MiB"
	case b >= kb:
		return strconv.FormatFloat(float64(b)/kb, 'f', 2, 64) + " KiB"
	default:
		return strconv.FormatInt(b, 10) + " B"
	}
}

func formatInUse(names []string) string {
	if len(names) == 0 {
		return "—"
	}
	return strings.Join(names, ", ")
}

func formatCreated(iso string) string {
	if iso == "" {
		return "—"
	}
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return iso
	}
	return t.Local().Format("2006-01-02 15:04")
}

func fallback(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func newVolumesSizeCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "size <name>",
		Short:   "Measure the disk usage of a volume",
		Example: `ore volumes size playground_lobby_world`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			local, specPath, remote, err := resolveMode(cmd)
			if err != nil {
				return err
			}
			if remote != nil {
				defer func() { _ = remote.Close() }()
			}
			if local {
				return localVolumeMeasure(cmd.Context(), specPath, args[0])
			}
			return remote.VolumeMeasure(cmd.Context(), args[0])
		},
	}
}

func newVolumesRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a volume",
		Example: `ore volumes remove playground_lobby_world
ore volumes remove playground_lobby_world --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			local, specPath, remote, err := resolveMode(cmd)
			if err != nil {
				return err
			}
			if remote != nil {
				defer func() { _ = remote.Close() }()
			}
			if local {
				return localVolumeRemove(cmd.Context(), specPath, args[0], force)
			}
			return remote.VolumeRemove(cmd.Context(), args[0], force)
		},
	}
	cmd.Flags().Bool("force", false, "stop containers that are using the volume before removing")
	return cmd
}

func newVolumesPruneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove volumes no longer declared in ore.yaml",
		Example: `ore volumes prune
ore volumes prune --dry-run`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			local, specPath, remote, err := resolveMode(cmd)
			if err != nil {
				return err
			}
			if remote != nil {
				defer func() { _ = remote.Close() }()
			}

			var report *volumes.PruneReport
			if local {
				report, err = localVolumePrune(cmd.Context(), specPath, dryRun)
			} else {
				report, err = remote.VolumePrune(cmd.Context(), dryRun)
			}
			if err != nil {
				return err
			}
			return printPruneReport(report)
		},
	}
	cmd.Flags().Bool("dry-run", false, "preview which volumes would be deleted without deleting them")
	return cmd
}

func localVolumeMeasure(ctx context.Context, specPath, name string) error {
	s, err := spec.Load(specPath)
	if err != nil {
		return err
	}
	svc, cleanup, err := localVolumeService(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	v, err := svc.Inspect(ctx, name)
	if err != nil {
		if errors.Is(err, volumes.ErrNotFound) {
			return fmt.Errorf("volume %q not found", name)
		}
		return err
	}
	if v.Project != s.Network {
		return fmt.Errorf("volume %q belongs to a different project", name)
	}

	size, err := svc.Measure(ctx, name)
	if err != nil {
		return err
	}
	fmt.Printf("%s\t%s (%d bytes)\n", name, formatSize(size), size)
	return nil
}

func localVolumeRemove(ctx context.Context, specPath, name string, force bool) error {
	s, err := spec.Load(specPath)
	if err != nil {
		return err
	}
	svc, cleanup, err := localVolumeService(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	v, err := svc.Inspect(ctx, name)
	if err != nil {
		if errors.Is(err, volumes.ErrNotFound) {
			return fmt.Errorf("volume %q not found", name)
		}
		return err
	}
	if v.Project != s.Network {
		return fmt.Errorf("volume %q belongs to a different project", name)
	}

	if err := svc.Remove(ctx, name, force); err != nil {
		if errors.Is(err, volumes.ErrInUse) {
			return fmt.Errorf("%w (retry with --force to stop containers first)", err)
		}
		return err
	}
	fmt.Printf("removed %s\n", name)
	return nil
}

func localVolumePrune(ctx context.Context, specPath string, dryRun bool) (*volumes.PruneReport, error) {
	s, err := spec.Load(specPath)
	if err != nil {
		return nil, err
	}
	svc, cleanup, err := localVolumeService(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	return svc.Prune(ctx, s.Network, deploy.DeclaredVolumeNames(s), dryRun)
}

func printPruneReport(report *volumes.PruneReport) error {
	if len(report.Candidates) == 0 {
		fmt.Println("no orphaned volumes")
		return nil
	}
	if report.DryRun {
		fmt.Printf("Dry-run: %d volume(s) would be removed:\n", len(report.Candidates))
	} else {
		fmt.Printf("Removed %d volume(s):\n", len(report.Deleted))
	}
	for _, c := range report.Candidates {
		fmt.Printf("  %s (owner %s, logical %s)\n", c.Name, fallback(c.Owner, "—"), fallback(c.Logical, "—"))
	}
	for _, sk := range report.Skipped {
		fmt.Printf("  skipped %s: %s\n", sk.Name, sk.Reason)
	}
	return nil
}
