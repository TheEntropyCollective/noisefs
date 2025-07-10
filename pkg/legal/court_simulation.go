package legal

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// CourtSimulator simulates legal proceedings and predicts outcomes
type CourtSimulator struct {
	precedentDB      *PrecedentDatabase
	argumentEvaluator *ArgumentEvaluator
	outcomePredictor  *OutcomePredictor
	jurisdictions     map[string]*JurisdictionRules
}

// NewCourtSimulator creates a new court simulator
func NewCourtSimulator() *CourtSimulator {
	sim := &CourtSimulator{
		precedentDB:      NewPrecedentDatabase(),
		argumentEvaluator: NewArgumentEvaluator(),
		outcomePredictor:  NewOutcomePredictor(),
		jurisdictions:     make(map[string]*JurisdictionRules),
	}
	
	// Initialize jurisdiction rules
	sim.initializeJurisdictions()
	
	return sim
}

// SimulateCase runs a comprehensive simulation of a legal case
func (cs *CourtSimulator) SimulateCase(testCase *TestCase) (*SimulationResult, error) {
	// Get jurisdiction rules
	jurisdiction, exists := cs.jurisdictions[testCase.Jurisdiction]
	if !exists {
		return nil, fmt.Errorf("unknown jurisdiction: %s", testCase.Jurisdiction)
	}
	
	result := &SimulationResult{
		CaseID:        testCase.ID,
		StartTime:     time.Now(),
		Jurisdiction:  testCase.Jurisdiction,
		Proceedings:   make([]*Proceeding, 0),
		Arguments:     make([]*ArgumentResult, 0),
		PrecedentRefs: make([]*PrecedentReference, 0),
	}
	
	// Phase 1: Pre-trial motions
	cs.simulatePreTrialPhase(testCase, jurisdiction, result)
	
	// Phase 2: Discovery
	cs.simulateDiscoveryPhase(testCase, jurisdiction, result)
	
	// Phase 3: Trial arguments
	cs.simulateTrialPhase(testCase, jurisdiction, result)
	
	// Phase 4: Verdict prediction
	cs.simulateVerdictPhase(testCase, jurisdiction, result)
	
	// Phase 5: Appeals analysis
	cs.simulateAppealsPhase(testCase, jurisdiction, result)
	
	// Calculate final outcome
	cs.calculateFinalOutcome(result)
	
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	
	return result, nil
}

// SimulationResult contains the results of a case simulation
type SimulationResult struct {
	CaseID          string                 `json:"case_id"`
	StartTime       time.Time              `json:"start_time"`
	EndTime         time.Time              `json:"end_time"`
	Duration        time.Duration          `json:"duration"`
	Jurisdiction    string                 `json:"jurisdiction"`
	
	// Procedural history
	Proceedings     []*Proceeding          `json:"proceedings"`
	
	// Legal arguments and their evaluations
	Arguments       []*ArgumentResult      `json:"arguments"`
	
	// Precedent analysis
	PrecedentRefs   []*PrecedentReference  `json:"precedent_refs"`
	
	// Predicted outcomes at each stage
	PreTrialOutcome *StageOutcome          `json:"pre_trial_outcome"`
	TrialOutcome    *StageOutcome          `json:"trial_outcome"`
	AppealOutcome   *StageOutcome          `json:"appeal_outcome,omitempty"`
	
	// Final prediction
	FinalOutcome    *FinalOutcome          `json:"final_outcome"`
	
	// Risk assessment
	RiskAssessment  *RiskAssessment        `json:"risk_assessment"`
	RiskScore       float64                `json:"risk_score"`
	
	// Recommendations
	Recommendations []string               `json:"recommendations"`
}

// Proceeding represents a legal proceeding in the case
type Proceeding struct {
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Date        time.Time `json:"date"`
	Outcome     string    `json:"outcome"`
	Impact      float64   `json:"impact"` // -1.0 to 1.0
}

// ArgumentResult represents the evaluation of a legal argument
type ArgumentResult struct {
	Argument       string    `json:"argument"`
	Type           string    `json:"type"`
	Side           string    `json:"side"` // "plaintiff" or "defendant"
	Strength       float64   `json:"strength"` // 0.0 to 1.0
	Precedents     []string  `json:"precedents"`
	Weaknesses     []string  `json:"weaknesses"`
	CounterArgs    []string  `json:"counter_arguments"`
	JudgeReception float64   `json:"judge_reception"` // 0.0 to 1.0
}

// PrecedentReference represents a relevant legal precedent
type PrecedentReference struct {
	CaseName      string   `json:"case_name"`
	Citation      string   `json:"citation"`
	Year          int      `json:"year"`
	Relevance     float64  `json:"relevance"` // 0.0 to 1.0
	Favorable     bool     `json:"favorable"`
	KeyPrinciples []string `json:"key_principles"`
	Application   string   `json:"application"`
}

// StageOutcome represents the predicted outcome at a specific stage
type StageOutcome struct {
	Stage          string    `json:"stage"`
	PredictedResult string   `json:"predicted_result"`
	Confidence     float64   `json:"confidence"`
	KeyFactors     []string  `json:"key_factors"`
	Alternatives   map[string]float64 `json:"alternatives"`
}

// FinalOutcome represents the final predicted outcome
type FinalOutcome struct {
	Result           string    `json:"result"`
	Success          bool      `json:"success"` // True if defendant wins
	Confidence       float64   `json:"confidence"`
	EstimatedDamages float64   `json:"estimated_damages,omitempty"`
	EstimatedCost    float64   `json:"estimated_cost"`
	EstimatedDuration int      `json:"estimated_duration_days"`
	SettlementRange  *SettlementRange `json:"settlement_range,omitempty"`
	AppealLikelihood float64   `json:"appeal_likelihood"`
}

// SettlementRange represents potential settlement amounts
type SettlementRange struct {
	Low    float64 `json:"low"`
	Medium float64 `json:"medium"`
	High   float64 `json:"high"`
	Recommended float64 `json:"recommended"`
}

// RiskAssessment provides risk analysis
type RiskAssessment struct {
	OverallRisk     string             `json:"overall_risk"` // "low", "medium", "high", "critical"
	LegalRisks      []RiskFactor       `json:"legal_risks"`
	FinancialRisks  []RiskFactor       `json:"financial_risks"`
	ReputationalRisks []RiskFactor     `json:"reputational_risks"`
	MitigationSteps []string           `json:"mitigation_steps"`
}

// RiskFactor represents a specific risk
type RiskFactor struct {
	Description string  `json:"description"`
	Probability float64 `json:"probability"` // 0.0 to 1.0
	Impact      string  `json:"impact"`      // "low", "medium", "high"
	Mitigation  string  `json:"mitigation"`
}

// JurisdictionRules defines rules for a specific jurisdiction
type JurisdictionRules struct {
	Name               string
	CopyrightStrength  float64 // How strongly copyright is protected
	FairUseStrength    float64 // How broadly fair use is interpreted
	PrivacyProtection  float64 // Privacy protection level
	TechSavviness      float64 // Understanding of technology
	PrecedentWeight    float64 // How much precedent matters
	StatutoryDamages   map[string]DamageRange
	FilingFees         float64
	TypicalDuration    int // Days
}

// DamageRange defines damage ranges for different violations
type DamageRange struct {
	Min float64
	Max float64
}

// Phase simulation methods

func (cs *CourtSimulator) simulatePreTrialPhase(testCase *TestCase, jurisdiction *JurisdictionRules, result *SimulationResult) {
	// Motion to dismiss
	motionToDismiss := &Proceeding{
		Type:        "motion_to_dismiss",
		Description: "Defendant files motion to dismiss for failure to state a claim",
		Date:        time.Now().Add(30 * 24 * time.Hour),
	}
	
	// Evaluate motion to dismiss
	dismissalChance := cs.evaluateDismissalChance(testCase, jurisdiction)
	
	if dismissalChance > 0.6 {
		motionToDismiss.Outcome = "granted"
		motionToDismiss.Impact = 1.0
		result.PreTrialOutcome = &StageOutcome{
			Stage:           "pre_trial",
			PredictedResult: "dismissed",
			Confidence:      dismissalChance,
			KeyFactors: []string{
				"Lack of copyright in individual blocks",
				"Technical impossibility of infringement",
				"Safe harbor protections apply",
			},
		}
	} else {
		motionToDismiss.Outcome = "denied"
		motionToDismiss.Impact = -0.2
		result.PreTrialOutcome = &StageOutcome{
			Stage:           "pre_trial",
			PredictedResult: "proceed_to_trial",
			Confidence:      1.0 - dismissalChance,
			KeyFactors: []string{
				"Factual disputes exist",
				"Novel legal questions",
				"Discovery needed",
			},
		}
	}
	
	result.Proceedings = append(result.Proceedings, motionToDismiss)
	
	// Preliminary injunction
	if testCase.Type == "dmca" && dismissalChance < 0.6 {
		injunction := &Proceeding{
			Type:        "preliminary_injunction",
			Description: "Plaintiff seeks preliminary injunction",
			Date:        time.Now().Add(45 * 24 * time.Hour),
		}
		
		injunctionChance := cs.evaluateInjunctionChance(testCase, jurisdiction)
		if injunctionChance > 0.5 {
			injunction.Outcome = "granted_partial"
			injunction.Impact = -0.3
		} else {
			injunction.Outcome = "denied"
			injunction.Impact = 0.2
		}
		
		result.Proceedings = append(result.Proceedings, injunction)
	}
}

func (cs *CourtSimulator) simulateDiscoveryPhase(testCase *TestCase, jurisdiction *JurisdictionRules, result *SimulationResult) {
	// Technical discovery
	techDiscovery := &Proceeding{
		Type:        "technical_discovery",
		Description: "Exchange of technical information and expert reports",
		Date:        time.Now().Add(90 * 24 * time.Hour),
		Outcome:     "completed",
		Impact:      0.1, // Generally favorable for defense
	}
	result.Proceedings = append(result.Proceedings, techDiscovery)
	
	// Depositions
	depositions := &Proceeding{
		Type:        "depositions",
		Description: "Depositions of key technical and business personnel",
		Date:        time.Now().Add(120 * 24 * time.Hour),
		Outcome:     "completed",
		Impact:      0.05,
	}
	result.Proceedings = append(result.Proceedings, depositions)
	
	// Discovery disputes
	if testCase.RiskLevel == "high" {
		dispute := &Proceeding{
			Type:        "discovery_dispute",
			Description: "Motion to compel production of user data",
			Date:        time.Now().Add(100 * 24 * time.Hour),
			Outcome:     "denied",
			Impact:      0.15, // Favorable for privacy
		}
		result.Proceedings = append(result.Proceedings, dispute)
	}
}

func (cs *CourtSimulator) simulateTrialPhase(testCase *TestCase, jurisdiction *JurisdictionRules, result *SimulationResult) {
	// Evaluate main arguments
	
	// Defense arguments
	if testCase.Type == "dmca" {
		// Block non-copyrightability argument
		blockArg := cs.evaluateArgument(
			"Individual blocks cannot be copyrighted",
			"copyright_theory",
			"defendant",
			testCase,
			jurisdiction,
		)
		result.Arguments = append(result.Arguments, blockArg)
		
		// Public domain mixing argument
		publicDomainArg := cs.evaluateArgument(
			"Substantial public domain content prevents copyright claims",
			"copyright_theory",
			"defendant",
			testCase,
			jurisdiction,
		)
		result.Arguments = append(result.Arguments, publicDomainArg)
		
		// Safe harbor argument
		safeHarborArg := cs.evaluateArgument(
			"DMCA safe harbor protections apply",
			"statutory_defense",
			"defendant",
			testCase,
			jurisdiction,
		)
		result.Arguments = append(result.Arguments, safeHarborArg)
	}
	
	// Plaintiff arguments
	if testCase.Type == "dmca" && testCase.DMCADetails != nil {
		// Direct infringement argument
		infringementArg := cs.evaluateArgument(
			"System facilitates copyright infringement",
			"infringement_theory",
			"plaintiff",
			testCase,
			jurisdiction,
		)
		result.Arguments = append(result.Arguments, infringementArg)
		
		// Contributory liability argument
		contributoryArg := cs.evaluateArgument(
			"Defendant has knowledge and contributes to infringement",
			"secondary_liability",
			"plaintiff",
			testCase,
			jurisdiction,
		)
		result.Arguments = append(result.Arguments, contributoryArg)
	}
	
	// Find relevant precedents
	cs.findRelevantPrecedents(testCase, result)
	
	// Predict trial outcome
	result.TrialOutcome = cs.predictTrialOutcome(testCase, result, jurisdiction)
}

func (cs *CourtSimulator) simulateVerdictPhase(testCase *TestCase, jurisdiction *JurisdictionRules, result *SimulationResult) {
	// Calculate verdict based on trial arguments
	defenseScore := 0.0
	plaintiffScore := 0.0
	
	for _, arg := range result.Arguments {
		if arg.Side == "defendant" {
			defenseScore += arg.Strength
		} else {
			plaintiffScore += arg.Strength
		}
	}
	
	// Normalize scores
	totalScore := defenseScore + plaintiffScore
	if totalScore > 0 {
		defenseScore /= totalScore
		plaintiffScore /= totalScore
	}
	
	// Apply jurisdiction bias
	defenseScore *= (1.0 + jurisdiction.TechSavviness * 0.2)
	plaintiffScore *= (1.0 + jurisdiction.CopyrightStrength * 0.2)
	
	// Create verdict outcome
	outcome := &StageOutcome{
		Stage:        "verdict",
		Confidence:   math.Max(defenseScore, plaintiffScore),
		Alternatives: make(map[string]float64),
	}
	
	if defenseScore > plaintiffScore {
		outcome.PredictedResult = "defendant_wins"
	} else {
		outcome.PredictedResult = "plaintiff_wins"
	}
	
	outcome.Alternatives["defendant_wins"] = defenseScore
	outcome.Alternatives["plaintiff_wins"] = plaintiffScore
	
	result.TrialOutcome = outcome
	
	// Calculate risk score
	result.RiskScore = plaintiffScore * 0.8 + (1.0 - outcome.Confidence) * 0.2
}

func (cs *CourtSimulator) simulateAppealsPhase(testCase *TestCase, jurisdiction *JurisdictionRules, result *SimulationResult) {
	// Determine if appeal is likely
	appealChance := cs.calculateAppealLikelihood(result.TrialOutcome, testCase)
	
	if appealChance > 0.3 {
		appeal := &Proceeding{
			Type:        "appeal_filed",
			Description: "Losing party files appeal to circuit court",
			Date:        time.Now().Add(200 * 24 * time.Hour),
			Outcome:     "pending",
			Impact:      0.0,
		}
		result.Proceedings = append(result.Proceedings, appeal)
		
		// Predict appeal outcome
		result.AppealOutcome = cs.predictAppealOutcome(testCase, result, jurisdiction)
	}
}

// Evaluation methods

func (cs *CourtSimulator) evaluateDismissalChance(testCase *TestCase, jurisdiction *JurisdictionRules) float64 {
	chance := 0.3 // Base chance
	
	if testCase.Type == "dmca" && testCase.DMCADetails != nil {
		// Increase dismissal chance for weak claims
		if testCase.DMCADetails.ClaimValidity == "invalid" {
			chance += 0.4
		} else if testCase.DMCADetails.ClaimValidity == "questionable" {
			chance += 0.2
		}
		
		// Technical defenses increase dismissal chance
		if testCase.DMCADetails.PublicDomainMix > 0.5 {
			chance += 0.15
		}
		
		// Descriptor-only claims are easier to dismiss
		if testCase.DMCADetails.DescriptorOnly {
			chance += 0.1
		}
		
		// Deficient notices increase dismissal
		if testCase.DMCADetails.NoticeQuality == "deficient" {
			chance += 0.2
		}
	}
	
	// Jurisdiction factors
	chance *= jurisdiction.TechSavviness // Tech-savvy courts more likely to understand defenses
	
	return math.Min(chance, 0.9)
}

func (cs *CourtSimulator) evaluateInjunctionChance(testCase *TestCase, jurisdiction *JurisdictionRules) float64 {
	chance := 0.2 // Base chance
	
	if testCase.Type == "dmca" && testCase.DMCADetails != nil {
		// Valid claims increase injunction chance
		if testCase.DMCADetails.ClaimValidity == "valid" {
			chance += 0.3
		}
		
		// High damages increase chance
		if testCase.DMCADetails.ClaimedDamages > 100000 {
			chance += 0.1
		}
		
		// Technical defenses reduce chance
		if testCase.DMCADetails.BlocksShared > 100 {
			chance -= 0.2 // Hard to enjoin widely shared blocks
		}
		
		if testCase.DMCADetails.DescriptorOnly {
			chance -= 0.1 // Already complying via descriptor takedown
		}
	}
	
	// Jurisdiction factors
	chance *= jurisdiction.CopyrightStrength
	
	return math.Max(math.Min(chance, 0.8), 0.0)
}

func (cs *CourtSimulator) evaluateArgument(
	argument string,
	argType string,
	side string,
	testCase *TestCase,
	jurisdiction *JurisdictionRules,
) *ArgumentResult {
	
	result := &ArgumentResult{
		Argument: argument,
		Type:     argType,
		Side:     side,
	}
	
	// Base strength depends on argument type and jurisdiction
	baseStrength := 0.5
	
	switch argType {
	case "copyright_theory":
		if side == "defendant" {
			baseStrength = 0.6 + (jurisdiction.TechSavviness * 0.2)
		} else {
			baseStrength = 0.4 + (jurisdiction.CopyrightStrength * 0.3)
		}
		
	case "statutory_defense":
		baseStrength = 0.7 // Strong statutory defenses
		
	case "infringement_theory":
		baseStrength = 0.5 * jurisdiction.CopyrightStrength
		
	case "secondary_liability":
		baseStrength = 0.4 // Harder to prove
	}
	
	// Adjust based on case specifics
	if testCase.Type == "dmca" && testCase.DMCADetails != nil {
		if strings.Contains(argument, "blocks cannot be copyrighted") {
			baseStrength += 0.2 * jurisdiction.TechSavviness
			result.Precedents = append(result.Precedents, "Feist Publications v. Rural Telephone Service")
			
			if testCase.DMCADetails.PublicDomainMix > 0.5 {
				baseStrength += 0.1
			}
		}
		
		if strings.Contains(argument, "public domain") {
			baseStrength += 0.15 * testCase.DMCADetails.PublicDomainMix
			result.Precedents = append(result.Precedents, "Eldred v. Ashcroft")
		}
		
		if strings.Contains(argument, "safe harbor") {
			if testCase.DMCADetails.DescriptorOnly {
				baseStrength += 0.15
			}
			result.Precedents = append(result.Precedents, "Viacom v. YouTube")
		}
	}
	
	result.Strength = math.Min(baseStrength, 0.95)
	
	// Judge reception based on jurisdiction
	result.JudgeReception = result.Strength * jurisdiction.TechSavviness
	
	// Add weaknesses
	if result.Strength < 0.6 {
		result.Weaknesses = append(result.Weaknesses, "Argument faces skepticism in this jurisdiction")
	}
	
	return result
}

func (cs *CourtSimulator) findRelevantPrecedents(testCase *TestCase, result *SimulationResult) {
	// Get relevant precedents from database
	precedents := cs.precedentDB.FindRelevantPrecedents(testCase)
	
	for _, p := range precedents {
		ref := &PrecedentReference{
			CaseName:    p.Name,
			Citation:    p.Citation,
			Year:        p.Year,
			Relevance:   p.CalculateRelevance(testCase),
			Favorable:   p.IsFavorable(testCase),
			KeyPrinciples: p.Principles,
		}
		
		// Determine how precedent applies
		if testCase.Type == "dmca" {
			if strings.Contains(p.Name, "Feist") {
				ref.Application = "Supports argument that blocks lack sufficient originality for copyright"
			} else if strings.Contains(p.Name, "Sony") {
				ref.Application = "Technology with substantial non-infringing uses is protected"
			} else if strings.Contains(p.Name, "Viacom") {
				ref.Application = "Safe harbor protections for service providers"
			}
		}
		
		result.PrecedentRefs = append(result.PrecedentRefs, ref)
	}
}

func (cs *CourtSimulator) predictTrialOutcome(testCase *TestCase, result *SimulationResult, jurisdiction *JurisdictionRules) *StageOutcome {
	// Calculate overall case strength
	defenseStrength := 0.0
	plaintiffStrength := 0.0
	
	for _, arg := range result.Arguments {
		if arg.Side == "defendant" {
			defenseStrength += arg.Strength * arg.JudgeReception
		} else {
			plaintiffStrength += arg.Strength * arg.JudgeReception
		}
	}
	
	// Factor in precedents
	favorablePrecedents := 0
	unfavorablePrecedents := 0
	
	for _, p := range result.PrecedentRefs {
		if p.Favorable {
			favorablePrecedents++
			defenseStrength += p.Relevance * 0.1
		} else {
			unfavorablePrecedents++
			plaintiffStrength += p.Relevance * 0.1
		}
	}
	
	// Normalize strengths
	totalStrength := defenseStrength + plaintiffStrength
	if totalStrength > 0 {
		defenseStrength /= totalStrength
		plaintiffStrength /= totalStrength
	}
	
	// Determine outcome
	outcome := &StageOutcome{
		Stage:       "trial",
		Alternatives: make(map[string]float64),
	}
	
	if defenseStrength > 0.6 {
		outcome.PredictedResult = "defendant_wins"
		outcome.Confidence = defenseStrength
		outcome.KeyFactors = []string{
			"Strong technical defenses",
			"Favorable precedents",
			"Lack of copyright in blocks",
		}
	} else if plaintiffStrength > 0.6 {
		outcome.PredictedResult = "plaintiff_wins"
		outcome.Confidence = plaintiffStrength
		outcome.KeyFactors = []string{
			"Evidence of infringement",
			"Secondary liability theories",
			"Jurisdiction favors copyright holders",
		}
	} else {
		outcome.PredictedResult = "mixed_verdict"
		outcome.Confidence = 0.5
		outcome.KeyFactors = []string{
			"Close case",
			"Factual disputes",
			"Novel legal issues",
		}
	}
	
	outcome.Alternatives["defendant_wins"] = defenseStrength
	outcome.Alternatives["plaintiff_wins"] = plaintiffStrength
	outcome.Alternatives["settlement"] = 1.0 - math.Abs(defenseStrength-plaintiffStrength)
	
	return outcome
}

func (cs *CourtSimulator) predictAppealOutcome(testCase *TestCase, result *SimulationResult, jurisdiction *JurisdictionRules) *StageOutcome {
	// Appeals focus more on legal issues than factual ones
	outcome := &StageOutcome{
		Stage:        "appeal",
		Alternatives: make(map[string]float64),
	}
	
	// Base appeal success rate
	reverseChance := 0.15 // Most trial verdicts are upheld
	
	// Novel legal issues increase reversal chance
	if testCase.Type == "dmca" {
		reverseChance += 0.2 // Novel technology law issues
	}
	
	// Strong legal arguments increase reversal chance
	strongLegalArgs := 0
	for _, arg := range result.Arguments {
		if arg.Type == "copyright_theory" || arg.Type == "statutory_defense" {
			if arg.Strength > 0.7 {
				strongLegalArgs++
			}
		}
	}
	reverseChance += float64(strongLegalArgs) * 0.1
	
	// Circuit split increases Supreme Court chance
	certChance := 0.02 // Base chance of cert
	if testCase.Type == "dmca" {
		certChance = 0.05 // Higher for novel issues
	}
	
	if reverseChance > 0.5 {
		outcome.PredictedResult = "reversed"
		outcome.Confidence = reverseChance
		outcome.KeyFactors = []string{
			"Novel legal questions",
			"Strong appellate arguments",
			"Circuit precedent favorable",
		}
	} else {
		outcome.PredictedResult = "affirmed"
		outcome.Confidence = 1.0 - reverseChance
		outcome.KeyFactors = []string{
			"Trial court discretion",
			"Factual findings upheld",
			"Standard of review favors affirmance",
		}
	}
	
	outcome.Alternatives["affirmed"] = 1.0 - reverseChance
	outcome.Alternatives["reversed"] = reverseChance
	outcome.Alternatives["supreme_court"] = certChance
	
	return outcome
}

func (cs *CourtSimulator) calculateAppealLikelihood(trialOutcome *StageOutcome, testCase *TestCase) float64 {
	if trialOutcome == nil {
		return 0.0
	}
	
	// Close cases more likely to be appealed
	likelihood := 1.0 - trialOutcome.Confidence
	
	// High stakes increase appeal likelihood
	if testCase.Type == "dmca" && testCase.DMCADetails != nil {
		if testCase.DMCADetails.ClaimedDamages > 100000 {
			likelihood += 0.2
		}
	}
	
	// Novel issues increase appeal likelihood
	if testCase.RiskLevel == "high" {
		likelihood += 0.15
	}
	
	return math.Min(likelihood, 0.8)
}

func (cs *CourtSimulator) calculateFinalOutcome(result *SimulationResult) {
	// Determine which outcome is final
	var relevantOutcome *StageOutcome
	
	if result.AppealOutcome != nil {
		relevantOutcome = result.AppealOutcome
	} else if result.TrialOutcome != nil {
		relevantOutcome = result.TrialOutcome
	} else {
		relevantOutcome = result.PreTrialOutcome
	}
	
	result.FinalOutcome = &FinalOutcome{
		Result:     relevantOutcome.PredictedResult,
		Success:    relevantOutcome.PredictedResult == "defendant_wins" || relevantOutcome.PredictedResult == "case_dismissed",
		Confidence: relevantOutcome.Confidence,
	}
	
	// Estimate costs based on proceedings
	baseCost := 50000.0
	for _, p := range result.Proceedings {
		switch p.Type {
		case "motion_to_dismiss":
			baseCost += 20000
		case "preliminary_injunction":
			baseCost += 30000
		case "technical_discovery":
			baseCost += 50000
		case "depositions":
			baseCost += 40000
		case "trial":
			baseCost += 100000
		case "appeal_filed":
			baseCost += 75000
		}
	}
	result.FinalOutcome.EstimatedCost = baseCost
	
	// Estimate duration
	result.FinalOutcome.EstimatedDuration = len(result.Proceedings) * 45 // Rough estimate
	
	// Settlement analysis
	if relevantOutcome.Alternatives["settlement"] > 0.3 {
		result.FinalOutcome.SettlementRange = cs.calculateSettlementRange(result)
	}
	
	// Risk assessment
	result.RiskAssessment = cs.assessRisks(result)
	
	// Generate recommendations
	result.Recommendations = cs.generateRecommendations(result)
}

func (cs *CourtSimulator) calculateSettlementRange(result *SimulationResult) *SettlementRange {
	// Base settlement on trial outcome probabilities
	plaintiffWinProb := result.TrialOutcome.Alternatives["plaintiff_wins"]
	
	// Expected value calculation
	maxDamages := 150000.0 // Default statutory max
	
	expectedValue := maxDamages * plaintiffWinProb
	
	return &SettlementRange{
		Low:         expectedValue * 0.3,
		Medium:      expectedValue * 0.5,
		High:        expectedValue * 0.8,
		Recommended: expectedValue * 0.5, // Recommend medium
	}
}

func (cs *CourtSimulator) assessRisks(result *SimulationResult) *RiskAssessment {
	assessment := &RiskAssessment{
		LegalRisks:        make([]RiskFactor, 0),
		FinancialRisks:    make([]RiskFactor, 0),
		ReputationalRisks: make([]RiskFactor, 0),
		MitigationSteps:   make([]string, 0),
	}
	
	// Determine overall risk
	if result.FinalOutcome.Confidence < 0.6 {
		assessment.OverallRisk = "high"
	} else if result.FinalOutcome.Confidence < 0.8 {
		assessment.OverallRisk = "medium"
	} else {
		assessment.OverallRisk = "low"
	}
	
	// Legal risks
	if result.FinalOutcome.Result != "defendant_wins" && result.FinalOutcome.Result != "dismissed" {
		assessment.LegalRisks = append(assessment.LegalRisks, RiskFactor{
			Description: "Adverse judgment risk",
			Probability: 1.0 - result.FinalOutcome.Confidence,
			Impact:      "high",
			Mitigation:  "Strengthen technical defenses",
		})
	}
	
	// Financial risks
	assessment.FinancialRisks = append(assessment.FinancialRisks, RiskFactor{
		Description: fmt.Sprintf("Legal costs estimated at $%.0f", result.FinalOutcome.EstimatedCost),
		Probability: 1.0,
		Impact:      "medium",
		Mitigation:  "Budget for legal defense fund",
	})
	
	// Reputational risks
	assessment.ReputationalRisks = append(assessment.ReputationalRisks, RiskFactor{
		Description: "Public perception of facilitating infringement",
		Probability: 0.3,
		Impact:      "medium",
		Mitigation:  "Public education about privacy technology",
	})
	
	// Mitigation steps
	assessment.MitigationSteps = []string{
		"Maintain comprehensive compliance documentation",
		"Implement robust repeat infringer policy",
		"Regular legal review of operations",
		"Consider insurance for legal costs",
		"Develop crisis communication plan",
	}
	
	return assessment
}

func (cs *CourtSimulator) generateRecommendations(result *SimulationResult) []string {
	recommendations := make([]string, 0)
	
	// Based on outcome
	switch result.FinalOutcome.Result {
	case "dismissed":
		recommendations = append(recommendations, "Maintain current legal strategy")
		recommendations = append(recommendations, "Document technical architecture thoroughly")
		
	case "defendant_wins":
		recommendations = append(recommendations, "Use verdict as precedent for future cases")
		recommendations = append(recommendations, "Consider publishing legal analysis")
		
	case "plaintiff_wins":
		recommendations = append(recommendations, "Immediate appeal recommended")
		recommendations = append(recommendations, "Review and strengthen technical defenses")
		recommendations = append(recommendations, "Consider architecture modifications")
		
	case "mixed_verdict":
		recommendations = append(recommendations, "Evaluate settlement options")
		recommendations = append(recommendations, "Prepare for appeal on key issues")
	}
	
	// General recommendations
	recommendations = append(recommendations, "Continue proactive DMCA compliance")
	recommendations = append(recommendations, "Maintain detailed audit logs")
	recommendations = append(recommendations, "Regular training on legal developments")
	
	return recommendations
}

// Initialize jurisdiction rules
func (cs *CourtSimulator) initializeJurisdictions() {
	// United States
	cs.jurisdictions["US"] = &JurisdictionRules{
		Name:              "United States",
		CopyrightStrength: 0.75,
		FairUseStrength:   0.6,
		PrivacyProtection: 0.5,
		TechSavviness:     0.6,
		PrecedentWeight:   0.8,
		StatutoryDamages: map[string]DamageRange{
			"copyright": {Min: 750, Max: 150000},
			"willful":   {Min: 150000, Max: 150000},
		},
		FilingFees:      400,
		TypicalDuration: 365,
	}
	
	// European Union
	cs.jurisdictions["EU"] = &JurisdictionRules{
		Name:              "European Union",
		CopyrightStrength: 0.7,
		FairUseStrength:   0.4, // More limited exceptions
		PrivacyProtection: 0.9, // GDPR
		TechSavviness:     0.7,
		PrecedentWeight:   0.6,
		StatutoryDamages: map[string]DamageRange{
			"copyright": {Min: 1000, Max: 100000},
		},
		FilingFees:      500,
		TypicalDuration: 400,
	}
	
	// Add more jurisdictions as needed
}

// RunBatchSimulation runs simulations on multiple test cases
func (cs *CourtSimulator) RunBatchSimulation(testCases []*TestCase) (*BatchSimulationResult, error) {
	results := make([]*SimulationResult, 0, len(testCases))
	
	for _, tc := range testCases {
		result, err := cs.SimulateCase(tc)
		if err != nil {
			return nil, fmt.Errorf("failed to simulate case %s: %w", tc.ID, err)
		}
		results = append(results, result)
	}
	
	// Analyze batch results
	batch := &BatchSimulationResult{
		TotalCases:     len(testCases),
		Results:        results,
		SuccessRate:    cs.calculateSuccessRate(results),
		RiskBreakdown:  cs.analyzeRiskBreakdown(results),
		CostAnalysis:   cs.analyzeCosts(results),
		TimeAnalysis:   cs.analyzeTimelines(results),
		Recommendations: cs.generateBatchRecommendations(results),
	}
	
	return batch, nil
}

// BatchSimulationResult contains aggregated results
type BatchSimulationResult struct {
	TotalCases      int                    `json:"total_cases"`
	Results         []*SimulationResult    `json:"results"`
	SuccessRate     float64                `json:"success_rate"`
	RiskBreakdown   map[string]int         `json:"risk_breakdown"`
	CostAnalysis    *CostAnalysis          `json:"cost_analysis"`
	TimeAnalysis    *TimeAnalysis          `json:"time_analysis"`
	Recommendations []string               `json:"recommendations"`
}

type CostAnalysis struct {
	TotalCost    float64 `json:"total_cost"`
	AverageCost  float64 `json:"average_cost"`
	MinCost      float64 `json:"min_cost"`
	MaxCost      float64 `json:"max_cost"`
	CostByType   map[string]float64 `json:"cost_by_type"`
}

type TimeAnalysis struct {
	AverageDuration int              `json:"average_duration_days"`
	MinDuration     int              `json:"min_duration_days"`
	MaxDuration     int              `json:"max_duration_days"`
	DurationByType  map[string]int   `json:"duration_by_type"`
}

func (cs *CourtSimulator) calculateSuccessRate(results []*SimulationResult) float64 {
	if len(results) == 0 {
		return 0.0
	}
	
	wins := 0
	for _, r := range results {
		if r.FinalOutcome != nil {
			if r.FinalOutcome.Result == "defendant_wins" || r.FinalOutcome.Result == "dismissed" {
				wins++
			}
		}
	}
	
	return float64(wins) / float64(len(results))
}

func (cs *CourtSimulator) analyzeRiskBreakdown(results []*SimulationResult) map[string]int {
	breakdown := make(map[string]int)
	
	for _, r := range results {
		if r.RiskAssessment != nil {
			breakdown[r.RiskAssessment.OverallRisk]++
		}
	}
	
	return breakdown
}

func (cs *CourtSimulator) analyzeCosts(results []*SimulationResult) *CostAnalysis {
	analysis := &CostAnalysis{
		CostByType: make(map[string]float64),
		MinCost:    math.MaxFloat64,
		MaxCost:    0,
	}
	
	typeCosts := make(map[string][]float64)
	
	for _, r := range results {
		if r.FinalOutcome != nil {
			cost := r.FinalOutcome.EstimatedCost
			analysis.TotalCost += cost
			
			if cost < analysis.MinCost {
				analysis.MinCost = cost
			}
			if cost > analysis.MaxCost {
				analysis.MaxCost = cost
			}
			
			// Track by case type
			caseType := "unknown"
			for _, p := range r.Proceedings {
				if p.Type == "motion_to_dismiss" {
					caseType = "dmca"
					break
				}
			}
			typeCosts[caseType] = append(typeCosts[caseType], cost)
		}
	}
	
	if len(results) > 0 {
		analysis.AverageCost = analysis.TotalCost / float64(len(results))
	}
	
	// Calculate average by type
	for caseType, costs := range typeCosts {
		sum := 0.0
		for _, c := range costs {
			sum += c
		}
		analysis.CostByType[caseType] = sum / float64(len(costs))
	}
	
	return analysis
}

func (cs *CourtSimulator) analyzeTimelines(results []*SimulationResult) *TimeAnalysis {
	analysis := &TimeAnalysis{
		DurationByType: make(map[string]int),
		MinDuration:    math.MaxInt32,
		MaxDuration:    0,
	}
	
	typeDurations := make(map[string][]int)
	totalDuration := 0
	
	for _, r := range results {
		if r.FinalOutcome != nil {
			duration := r.FinalOutcome.EstimatedDuration
			totalDuration += duration
			
			if duration < analysis.MinDuration {
				analysis.MinDuration = duration
			}
			if duration > analysis.MaxDuration {
				analysis.MaxDuration = duration
			}
			
			// Track by case type
			caseType := "unknown"
			for _, p := range r.Proceedings {
				if p.Type == "motion_to_dismiss" {
					caseType = "dmca"
					break
				}
			}
			typeDurations[caseType] = append(typeDurations[caseType], duration)
		}
	}
	
	if len(results) > 0 {
		analysis.AverageDuration = totalDuration / len(results)
	}
	
	// Calculate average by type
	for caseType, durations := range typeDurations {
		sum := 0
		for _, d := range durations {
			sum += d
		}
		analysis.DurationByType[caseType] = sum / len(durations)
	}
	
	return analysis
}

func (cs *CourtSimulator) generateBatchRecommendations(results []*SimulationResult) []string {
	recommendations := make([]string, 0)
	
	successRate := cs.calculateSuccessRate(results)
	
	if successRate > 0.8 {
		recommendations = append(recommendations, "Current legal position is strong")
		recommendations = append(recommendations, "Consider more aggressive stance on frivolous claims")
	} else if successRate < 0.5 {
		recommendations = append(recommendations, "Legal position needs strengthening")
		recommendations = append(recommendations, "Consider architecture modifications to reduce risk")
		recommendations = append(recommendations, "Increase legal budget reserves")
	}
	
	// Analyze common failure points
	failureReasons := make(map[string]int)
	for _, r := range results {
		if r.FinalOutcome != nil && r.FinalOutcome.Result != "defendant_wins" && r.FinalOutcome.Result != "dismissed" {
			for _, factor := range r.TrialOutcome.KeyFactors {
				failureReasons[factor]++
			}
		}
	}
	
	// Address most common failure reasons
	type reason struct {
		factor string
		count  int
	}
	var reasons []reason
	for f, c := range failureReasons {
		reasons = append(reasons, reason{f, c})
	}
	sort.Slice(reasons, func(i, j int) bool {
		return reasons[i].count > reasons[j].count
	})
	
	if len(reasons) > 0 {
		recommendations = append(recommendations, 
			fmt.Sprintf("Address common vulnerability: %s", reasons[0].factor))
	}
	
	// Cost recommendations
	costs := cs.analyzeCosts(results)
	if costs.AverageCost > 100000 {
		recommendations = append(recommendations, "Consider litigation insurance")
		recommendations = append(recommendations, "Develop early settlement strategies")
	}
	
	return recommendations
}