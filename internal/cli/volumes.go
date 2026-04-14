package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/server/dto"
	"github.com/oreforge/ore/internal/spec"
	"github.com/oreforge/ore/internal/volumes"
)

func newVolumesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "volumes",
		Aliases: []string{"volume", "vol"},
		Short:   "Manage ore-managed Docker volumes",
	}
	cmd.AddCommand(newVolumesListCmd(), newVolumesInspectCmd())
	return cmd
}

func newVolumesListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List volumes for the current project",
		Example: "ore volumes ls\n  ore volumes ls -o json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			output, _ := cmd.Flags().GetString("output")
			vols, err := fetchVolumes(cmd)
			if err != nil {
				return err
			}
			return renderVolumeList(vols, output)
		},
	}
	cmd.Flags().StringP("output", "o", "table", "output format: table|json")
	return cmd
}

func newVolumesInspectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "inspect <name>",
		Short:   "Show detailed information about a volume",
		Example: "ore volumes inspect playground_lobby_world",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			output, _ := cmd.Flags().GetString("output")
			v, err := fetchVolume(cmd, args[0])
			if err != nil {
				return err
			}
			return renderVolume(v, output)
		},
	}
	cmd.Flags().StringP("output", "o", "table", "output format: table|json")
	return cmd
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

func renderVolumeList(vols []dto.VolumeResponse, output string) error {
	switch strings.ToLower(output) {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(vols)
	case "", "table":
		return writeVolumeTable(vols)
	default:
		return fmt.Errorf("unknown output format: %s (valid: table, json)", output)
	}
}

func renderVolume(v *dto.VolumeResponse, output string) error {
	switch strings.ToLower(output) {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	case "", "table":
		return writeVolumeDetail(v)
	default:
		return fmt.Errorf("unknown output format: %s (valid: table, json)", output)
	}
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
