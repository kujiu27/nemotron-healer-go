package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kujiu27/nemotron-healer-go/internal/engine"
)

type SarifLocation struct {
	PhysicalLocation struct {
		ArtifactLocation struct {
			URI string `json:"uri"`
		} `json:"artifactLocation"`
		Region struct {
			StartLine int `json:"startLine"`
		} `json:"region"`
	} `json:"physicalLocation"`
}

type SarifResult struct {
	RuleID    string          `json:"ruleId"`
	RuleIndex int             `json:"ruleIndex"`
	Level     string          `json:"level"`
	Message   struct {
		Text string `json:"text"`
	} `json:"message"`
	Locations []SarifLocation `json:"locations,omitempty"`
}

type SarifRule struct {
	ID               string `json:"id"`
	ShortDescription struct {
		Text string `json:"text"`
	} `json:"shortDescription"`
	HelpURI string `json:"helpUri,omitempty"`
}

type SarifReport struct {
	Schema  string `json:"$schema"`
	Version string `json:"version"`
	Runs    []struct {
		Tool struct {
			Driver struct {
				Name           string      `json:"name"`
				Version        string      `json:"version"`
				InformationURI string      `json:"informationUri"`
				Rules          []SarifRule `json:"rules"`
			} `json:"driver"`
		} `json:"tool"`
		Results []SarifResult `json:"results"`
	} `json:"runs"`
}

// ExportSarif serializes the healing session defect & outcome to standard SARIF 2.1.0 JSON
func ExportSarif(session *engine.HealingSession, outputPath string) error {
	ruleID := "NH-DEFECT"
	ruleDesc := "Autonomous Code Self-Healing Defect Diagnostic"
	if session.DefectArchetype != "" {
		clean := regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(session.DefectArchetype, "-")
		clean = strings.Trim(clean, "-")
		ruleID = "NH-" + strings.ToUpper(clean)
		ruleDesc = fmt.Sprintf("Autonomous Diagnostic: %s", session.DefectArchetype)
	}
	level := "warning"
	msgText := "Defect identified and resolved autonomously by Nemotron-Healer"
	if session.PatchDigest != "" {
		msgText = fmt.Sprintf("Defect [%s] resolved autonomously by Nemotron-Healer (Audit Digest: %s)", session.DefectArchetype, session.PatchDigest)
	}
	if !session.IsResolved {
		level = "error"
		lastErr := session.LastError
		if lastErr == "" {
			lastErr = session.InitialError
		}
		if lastErr != "" {
			msgText = "Defect unresolved: " + lastErr
		} else {
			msgText = "Defect unresolved: autonomous healing budget exhausted"
		}
	}
	if len(msgText) > 1000 {
		msgText = msgText[:997] + "..."
	}

	var locations []SarifLocation
	if session.TargetFile != "" {
		loc := SarifLocation{}
		loc.PhysicalLocation.ArtifactLocation.URI = filepath.ToSlash(session.TargetFile)
		if session.TargetLine > 0 {
			loc.PhysicalLocation.Region.StartLine = session.TargetLine
		} else {
			loc.PhysicalLocation.Region.StartLine = 1
		}
		locations = append(locations, loc)
	}
	for _, aff := range session.BlastAffectedFiles {
		if aff != "" && aff != session.TargetFile {
			affLoc := SarifLocation{}
			affLoc.PhysicalLocation.ArtifactLocation.URI = filepath.ToSlash(aff)
			affLoc.PhysicalLocation.Region.StartLine = 1
			locations = append(locations, affLoc)
		}
	}

	report := SarifReport{
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
		Version: "2.1.0",
		Runs: []struct {
			Tool struct {
				Driver struct {
					Name           string      `json:"name"`
					Version        string      `json:"version"`
					InformationURI string      `json:"informationUri"`
					Rules          []SarifRule `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []SarifResult `json:"results"`
		}{
			{
				Tool: struct {
					Driver struct {
						Name           string      `json:"name"`
						Version        string      `json:"version"`
						InformationURI string      `json:"informationUri"`
						Rules          []SarifRule `json:"rules"`
					} `json:"driver"`
				}{
					Driver: struct {
						Name           string      `json:"name"`
						Version        string      `json:"version"`
						InformationURI string      `json:"informationUri"`
						Rules          []SarifRule `json:"rules"`
					}{
						Name:           "Nemotron-Healer",
						Version:        strings.TrimPrefix(Version, "v"),
						InformationURI: "https://github.com/kujiu27/nemotron-healer-go",
						Rules: []SarifRule{
							{
								ID:      ruleID,
								HelpURI: "https://github.com/kujiu27/nemotron-healer-go",
								ShortDescription: struct {
									Text string `json:"text"`
								}{
									Text: ruleDesc,
								},
							},
						},
					},
				},
				Results: []SarifResult{
					{
						RuleID:    ruleID,
						RuleIndex: 0,
						Level:     level,
						Locations: locations,
						Message: struct {
							Text string `json:"text"`
						}{
							Text: msgText,
						},
					},
				},
			},
		},
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(outputPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory for sarif report %q: %w", outputPath, err)
		}
	}
	return os.WriteFile(outputPath, data, 0644)
}
