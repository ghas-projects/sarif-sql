package models

// SARIF schema structs - defines the structure of SARIF files we process
// Only includes the fields we actually use to avoid unnecessary overhead

// SarifDocument represents the root SARIF document
type SarifDocument struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []SarifRun `json:"runs"`
}

// SarifRun represents a single run in the SARIF file
type SarifRun struct {
	Tool                     SarifTool         `json:"tool"`
	Results                  []SarifResult     `json:"results"`
	VersionControlProvenance []SarifProvenance `json:"versionControlProvenance,omitempty"`
}

// SarifTool represents the tool that produced the results
type SarifTool struct {
	Driver SarifDriver `json:"driver"`
}

// SarifDriver represents the tool driver information
type SarifDriver struct {
	Name            string      `json:"name"`
	SemanticVersion string      `json:"semanticVersion,omitempty"`
	Rules           []SarifRule `json:"rules,omitempty"`
}

// SarifRule represents a rule definition
type SarifRule struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// SarifResult represents a single result (alert/finding)
type SarifResult struct {
	RuleID             string            `json:"ruleId"`
	Message            SarifMessage      `json:"message"`
	Locations          []SarifLocation   `json:"locations,omitempty"`
	PartialFingerprint map[string]string `json:"partialFingerprints,omitempty"`
	CodeFlows          []SarifCodeFlow   `json:"codeFlows,omitempty"`
}

// SarifMessage represents a message in a result
type SarifMessage struct {
	Text string `json:"text"`
}

// SarifLocation represents a location in the source code
type SarifLocation struct {
	PhysicalLocation SarifPhysicalLocation `json:"physicalLocation"`
}

// SarifPhysicalLocation represents a physical location in a file
type SarifPhysicalLocation struct {
	ArtifactLocation SarifArtifactLocation `json:"artifactLocation"`
	Region           SarifRegion           `json:"region,omitempty"`
	ContextRegion    SarifRegion           `json:"contextRegion,omitempty"`
}

// SarifArtifactLocation represents a file location
type SarifArtifactLocation struct {
	URI string `json:"uri"`
}

// SarifRegion represents a region within a file
type SarifRegion struct {
	StartLine   int          `json:"startLine,omitempty"`
	StartColumn int          `json:"startColumn,omitempty"`
	EndLine     int          `json:"endLine,omitempty"`
	EndColumn   int          `json:"endColumn,omitempty"`
	Snippet     SarifSnippet `json:"snippet,omitempty"`
}

// SarifSnippet represents a code snippet
type SarifSnippet struct {
	Text string `json:"text"`
}

// SarifCodeFlow represents a code flow (for data flow analysis)
type SarifCodeFlow struct {
	ThreadFlows []SarifThreadFlow `json:"threadFlows"`
}

// SarifThreadFlow represents a thread flow
type SarifThreadFlow struct {
	Locations []SarifThreadFlowLocation `json:"locations"`
}

// SarifThreadFlowLocation represents a location in a thread flow
type SarifThreadFlowLocation struct {
	Location SarifLocation `json:"location"`
	Taxa     []SarifTaxa   `json:"taxa,omitempty"`
}

// SarifTaxa represents taxonomic information
type SarifTaxa struct {
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// SarifProvenance represents version control provenance information
type SarifProvenance struct {
	RepositoryURI string `json:"repositoryUri"`
}
