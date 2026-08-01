package ui

// aiProposalPageData is the complete no-JavaScript review surface from one
// API-generated ContextPack and Proposal. Every displayed value comes from
// those two API DTOs; the UI derives no engineering conclusion of its own.
type aiProposalPageData struct {
	PageTitle         string
	FeatureCardID     string
	ArtifactID        string
	Capability        apiProposalCapabilityDTO
	CurrentContent    apiContentDTO
	ProposedContent   apiContentDTO
	Requirements      []apiProposalRequirementDTO
	Claims            []apiProposalClaimDTO
	Decisions         []apiProposalDecisionDTO
	OpenQuestions     []apiProposalOpenQuestionDTO
	UncoveredCriteria []apiProposalUncoveredCriterionDTO
	Findings          []apiProposalFindingDTO
	ContextSources    []string
	ProposalSources   []string
	Rationale         string
	ContextDigest     string
	ProposalDigest    string
	CanonicalProposal string
}

func mapAIProposalPageData(featureCardID, artifactID string, contextPack apiProposalContextPackDTO, generated apiCapabilityProposalDTO, canonicalProposal string) aiProposalPageData {
	return aiProposalPageData{
		PageTitle: "Review AI-assisted proposal", FeatureCardID: featureCardID, ArtifactID: artifactID,
		Capability: contextPack.Capability, CurrentContent: contextPack.Capability.Content, ProposedContent: generated.Content,
		Requirements: contextPack.Requirements, Claims: contextPack.Claims, Decisions: contextPack.Decisions,
		OpenQuestions: contextPack.OpenQuestions, UncoveredCriteria: contextPack.UncoveredCriteria,
		Findings: contextPack.Findings, ContextSources: contextPack.Sources, ProposalSources: generated.Sources,
		Rationale: generated.Rationale, ContextDigest: generated.ContextDigest,
		ProposalDigest: generated.ProposalDigest, CanonicalProposal: canonicalProposal,
	}
}
