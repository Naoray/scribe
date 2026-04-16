package cmd

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/doctor"
)

func newDoctorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Inspect managed skill health",
		Long: `Inspect managed skill health and report canonical metadata or projection drift.

Use --json for machine-readable output.

Examples:
  scribe doctor
  scribe doctor --skill recap
  scribe doctor --json`,
		Args: cobra.NoArgs,
		RunE: runDoctor,
	}
	cmd.Flags().Bool("fix", false, "Reserved for repair mode (Task 4)")
	cmd.Flags().String("skill", "", "Inspect a single managed skill")
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	return cmd
}

type doctorIssueJSON struct {
	Skill   string `json:"skill"`
	Tool    string `json:"tool,omitempty"`
	Kind    string `json:"kind"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type doctorReportJSON struct {
	Skill  string            `json:"skill,omitempty"`
	Fix    bool              `json:"fix"`
	Issues []doctorIssueJSON `json:"issues"`
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	fixFlag, _ := cmd.Flags().GetBool("fix")
	skillFlag, _ := cmd.Flags().GetString("skill")
	jsonFlag, _ := cmd.Flags().GetBool("json")

	if fixFlag && jsonFlag {
		return fmt.Errorf("doctor: --fix cannot be combined with --json")
	}
	if fixFlag {
		return fmt.Errorf("doctor: --fix not implemented yet")
	}

	factory := newCommandFactory()

	cfg, err := factory.Config()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	st, err := factory.State()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	if skillFlag != "" {
		if _, ok := st.Installed[skillFlag]; !ok {
			return fmt.Errorf("doctor: skill %q is not installed", skillFlag)
		}
	}

	report, err := doctor.InspectManagedSkills(cfg, st, skillFlag)
	if err != nil {
		return fmt.Errorf("inspect managed skills: %w", err)
	}

	if jsonFlag {
		return writeDoctorJSON(cmd.OutOrStdout(), skillFlag, report)
	}
	return writeDoctorText(cmd.OutOrStdout(), skillFlag, report)
}

func writeDoctorJSON(w io.Writer, skill string, report doctor.Report) error {
	out := doctorReportJSON{
		Skill:  skill,
		Fix:    false,
		Issues: make([]doctorIssueJSON, 0, len(report.Issues)),
	}
	for _, issue := range report.Issues {
		out.Issues = append(out.Issues, doctorIssueJSON{
			Skill:   issue.Skill,
			Tool:    issue.Tool,
			Kind:    string(issue.Kind),
			Status:  issue.Status,
			Message: issue.Message,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func writeDoctorText(w io.Writer, skill string, report doctor.Report) error {
	if len(report.Issues) == 0 {
		if skill != "" {
			_, err := fmt.Fprintf(w, "No managed skill issues found for %s.\n", skill)
			return err
		}
		_, err := fmt.Fprintln(w, "No managed skill issues found.")
		return err
	}

	if skill != "" {
		if _, err := fmt.Fprintf(w, "Managed skill issues for %s:\n", skill); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(w, "Managed skill issues:"); err != nil {
			return err
		}
	}

	for _, issue := range report.Issues {
		if issue.Tool != "" {
			if _, err := fmt.Fprintf(w, "- %s [%s] %s tool=%s: %s\n", issue.Skill, issue.Status, issue.Kind, issue.Tool, issue.Message); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(w, "- %s [%s] %s: %s\n", issue.Skill, issue.Status, issue.Kind, issue.Message); err != nil {
			return err
		}
	}

	return nil
}
