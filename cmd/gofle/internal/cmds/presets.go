package cmds

import (
	"GoFLE/cmd/gofle/internal/shared"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// PresetsCmd manages shared search presets (search_presets.json).
var PresetsCmd = &cobra.Command{
	Use:   "presets",
	Short: "List or import search presets (shared with the desktop app)",
	Args:  UsageArgs(cobra.MaximumNArgs(0)),
	RunE:  runPresetsList,
}

var presetsImportCmd = &cobra.Command{
	Use:   "import <file.json>",
	Short: "Import presets from a GUI localStorage JSON export",
	Long: `Import search presets from a JSON file in the desktop localStorage format:

  [{"name":"errors","filter":{"exclude_patterns":["debug"],"nearby_query":"timeout",...}}]

Presets are merged into ~/.config/GoFLE/search_presets.json (same store as the
desktop app and the in-viewer P key). Existing names are replaced.`,
	Args: UsageArgs(cobra.ExactArgs(1)),
	RunE: runPresetsImport,
}

var presetsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved search presets",
	Args:  UsageArgs(cobra.MaximumNArgs(0)),
	RunE:  runPresetsList,
}

var presetsExportCmd = &cobra.Command{
	Use:   "export [file.json]",
	Short: "Export presets in GUI localStorage-compatible JSON",
	Long: `Write search presets to a JSON file (or stdout) in the format used by the
desktop app localStorage key gofle.searchPresets:

  [{"name":"…","filter":{…}}]

Default path is stdout when no file is given.`,
	Args: UsageArgs(cobra.MaximumNArgs(1)),
	RunE: runPresetsExport,
}

func init() {
	PresetsCmd.AddCommand(presetsImportCmd, presetsListCmd, presetsExportCmd)
}

func runPresetsList(cmd *cobra.Command, _ []string) error {
	cfg, err := shared.Config()
	if err != nil {
		return err
	}
	presets := cfg.GetSearchPresets()
	if len(presets) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No search presets.")
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Save from the viewer (P → s) or: gofle presets import <file.json>")
		return nil
	}
	for i, p := range presets {
		extra := ""
		if p.NearbyQuery != "" {
			extra += " near:" + p.NearbyQuery
		}
		if len(p.ExcludePatterns) > 0 {
			extra += fmt.Sprintf(" excl:%d", len(p.ExcludePatterns))
		}
		if len(p.Levels) > 0 {
			extra += " levels"
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%2d  %s%s\n", i+1, p.Name, extra)
	}
	return nil
}

func runPresetsImport(cmd *cobra.Command, args []string) error {
	cfg, err := shared.Config()
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(args[0]) //nolint:gosec // user-supplied path is intentional
	if err != nil {
		return fmt.Errorf("read %s: %w", args[0], err)
	}
	n, err := cfg.ImportGUIPresets(raw)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Imported %d preset(s) into search_presets.json\n", n)
	return nil
}

func runPresetsExport(cmd *cobra.Command, args []string) error {
	cfg, err := shared.Config()
	if err != nil {
		return err
	}
	presets := cfg.GetSearchPresets()
	type guiFilter struct {
		ExcludePatterns []string `json:"exclude_patterns,omitempty"`
		ExcludeIsRegex  bool     `json:"exclude_is_regex,omitempty"`
		NearbyQuery     string   `json:"nearby_query,omitempty"`
		NearbyMode      string   `json:"nearby_mode,omitempty"`
		NearbyWindow    int      `json:"nearby_window,omitempty"`
		Levels          []string `json:"levels,omitempty"`
		TimeFrom        string   `json:"time_from,omitempty"`
		TimeTo          string   `json:"time_to,omitempty"`
		TimeRequire     bool     `json:"time_require_stamp,omitempty"`
		LineStart       int64    `json:"line_start,omitempty"`
		LineEnd         int64    `json:"line_end,omitempty"`
		CtxBefore       int      `json:"ctx_before,omitempty"`
		CtxAfter        int      `json:"ctx_after,omitempty"`
		Query           string   `json:"query,omitempty"`
		SecondaryQuery  string   `json:"secondary_query,omitempty"`
		IsRegex         bool     `json:"is_regex,omitempty"`
		CaseSensitive   bool     `json:"case_sensitive,omitempty"`
		InvertMatch     bool     `json:"invert_match,omitempty"`
	}
	type guiItem struct {
		Name   string    `json:"name"`
		Filter guiFilter `json:"filter"`
	}
	out := make([]guiItem, 0, len(presets))
	for _, p := range presets {
		out = append(out, guiItem{
			Name: p.Name,
			Filter: guiFilter{
				ExcludePatterns: p.ExcludePatterns,
				ExcludeIsRegex:  p.ExcludeIsRegex,
				NearbyQuery:     p.NearbyQuery,
				NearbyMode:      p.NearbyMode,
				NearbyWindow:    p.NearbyWindow,
				Levels:          p.Levels,
				TimeFrom:        p.TimeFrom,
				TimeTo:          p.TimeTo,
				TimeRequire:     p.TimeRequire,
				LineStart:       p.LineStart,
				LineEnd:         p.LineEnd,
				CtxBefore:       p.CtxBefore,
				CtxAfter:        p.CtxAfter,
				Query:           p.Query,
				SecondaryQuery:  p.SecondaryQuery,
				IsRegex:         p.IsRegex,
				CaseSensitive:   p.CaseSensitive,
				InvertMatch:     p.InvertMatch,
			},
		})
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(args) == 0 {
		_, err = cmd.OutOrStdout().Write(data)
		return err
	}
	if err := os.WriteFile(args[0], data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", args[0], err)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Exported %d preset(s) to %s\n", len(out), args[0])
	return nil
}
