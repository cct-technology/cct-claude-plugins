package check

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/url"
	"path/filepath"
)

const sarifSchemaURL = "https://docs.oasis-open.org/sarif/sarif/v2.1.0/errata01/os/schemas/sarif-schema-2.1.0.json"

type sarifText struct {
	Text string `json:"text"`
}

type sarifRule struct {
	ID               string    `json:"id"`
	ShortDescription sarifText `json:"shortDescription"`
	FullDescription  sarifText `json:"fullDescription"`
	Help             sarifText `json:"help"`
}

type sarifRuleRef struct {
	ID    string `json:"id"`
	Index int    `json:"index"`
}

type sarifNotification struct {
	Level          string       `json:"level"`
	Message        sarifText    `json:"message"`
	AssociatedRule sarifRuleRef `json:"associatedRule"`
}

type sarifLocation struct {
	PhysicalLocation struct {
		ArtifactLocation struct {
			URI       string `json:"uri"`
			URIBaseID string `json:"uriBaseId"`
		} `json:"artifactLocation"`
		Region struct {
			StartLine int `json:"startLine"`
		} `json:"region"`
	} `json:"physicalLocation"`
}

type sarifSuppression struct {
	Kind          string            `json:"kind"`
	Status        string            `json:"status"`
	Justification string            `json:"justification"`
	Properties    map[string]string `json:"properties"`
}

type sarifResult struct {
	RuleID              string             `json:"ruleId"`
	RuleIndex           int                `json:"ruleIndex"`
	Level               string             `json:"level"`
	Message             sarifText          `json:"message"`
	Locations           []sarifLocation    `json:"locations"`
	PartialFingerprints map[string]string  `json:"partialFingerprints"`
	Suppressions        []sarifSuppression `json:"suppressions,omitempty"`
}

// WriteSARIF writes rep as a SARIF 2.1.0 log for GitHub code scanning and
// other SARIF readers. rules supplies each rule's description and fix;
// version is the qk version.
//
// Open findings are results: error when they block, warning when advisory.
// Waived findings are results with an external suppression. Checks that did
// not run are notifications, so a skipped check never reads as clean.
func WriteSARIF(w io.Writer, rep Report, rules []Rule, version string) error {
	byID := map[string]Rule{}
	for _, r := range rules {
		byID[r.ID] = r
	}
	sRules := []sarifRule{}
	results := []sarifResult{}
	notes := []sarifNotification{}
	for i, rr := range rep.Results {
		r, ok := byID[rr.Rule]
		if !ok { // not in the registry given: describe it from the result
			r = Rule{Title: rr.Title, Why: rr.Title, Fix: rr.Fix}
		}
		sRules = append(sRules, sarifRule{
			ID:               rr.Rule,
			ShortDescription: sarifText{rr.Title},
			FullDescription:  sarifText{r.Why},
			Help:             sarifText{r.Fix + " More: qk explain " + rr.Rule},
		})
		level := "warning"
		if rr.Status == Fail {
			level = "error"
		}
		for _, f := range rr.Findings {
			results = append(results, sarifFinding(rr.Rule, i, level, f.File, f.Message))
		}
		// A waived finding would have blocked only in enforced mode.
		waivedLevel := "warning"
		if rr.Mode == Enforced {
			waivedLevel = "error"
		}
		for _, wf := range rr.Waived {
			res := sarifFinding(rr.Rule, i, waivedLevel, wf.File, wf.Message)
			res.Suppressions = []sarifSuppression{{
				Kind: "external", Status: "accepted", Justification: wf.Reason,
				Properties: map[string]string{"owner": wf.Owner, "expires": wf.Expires},
			}}
			results = append(results, res)
		}
		if rr.Status == Skipped {
			notes = append(notes, sarifNotification{
				Level:          "warning",
				Message:        sarifText{rr.Rule + " did not run: " + rr.Reason},
				AssociatedRule: sarifRuleRef{ID: rr.Rule, Index: i},
			})
		}
	}

	type driver struct {
		Name           string      `json:"name"`
		Version        string      `json:"version"`
		InformationURI string      `json:"informationUri"`
		Rules          []sarifRule `json:"rules"`
	}
	type invocation struct {
		ExecutionSuccessful        bool                `json:"executionSuccessful"`
		ToolExecutionNotifications []sarifNotification `json:"toolExecutionNotifications"`
	}
	type run struct {
		Tool struct {
			Driver driver `json:"driver"`
		} `json:"tool"`
		Invocations []invocation  `json:"invocations"`
		Results     []sarifResult `json:"results"`
	}
	r := run{Invocations: []invocation{{true, notes}}, Results: results}
	r.Tool.Driver = driver{"Quality Kit", version, "https://github.com/rkvalvaa/cct-claude-plugins", sRules}
	log := struct {
		Schema  string `json:"$schema"`
		Version string `json:"version"`
		Runs    []run  `json:"runs"`
	}{sarifSchemaURL, "2.1.0", []run{r}}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(log)
}

func sarifFinding(rule string, index int, level, file, message string) sarifResult {
	// ponytail: findings have no line yet, so every result points at line 1.
	// GitHub shows a result only with a location; a finding about the repo as a
	// whole is placed on quality.yaml, the file that turned the check on.
	uri := "quality.yaml"
	if file != "" {
		uri = (&url.URL{Path: filepath.ToSlash(file)}).EscapedPath()
	}
	var loc sarifLocation
	loc.PhysicalLocation.ArtifactLocation.URI = uri
	loc.PhysicalLocation.ArtifactLocation.URIBaseID = "%SRCROOT%"
	loc.PhysicalLocation.Region.StartLine = 1
	// GitHub keys alerts on primaryLocationLineHash and computes it from the
	// line when it is missing, which would merge findings that share a file.
	sum := sha256.Sum256([]byte(rule + "\x00" + file + "\x00" + message))
	return sarifResult{
		RuleID: rule, RuleIndex: index, Level: level,
		Message:             sarifText{message},
		Locations:           []sarifLocation{loc},
		PartialFingerprints: map[string]string{"primaryLocationLineHash": hex.EncodeToString(sum[:16])},
	}
}
