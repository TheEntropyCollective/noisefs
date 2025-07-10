package legal

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// PrecedentDatabase stores and analyzes legal precedents
type PrecedentDatabase struct {
	precedents map[string]*Precedent
	indexes    map[string][]string // Multiple indexes for fast lookup
}

// Precedent represents a legal precedent case
type Precedent struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Citation     string                 `json:"citation"`
	Year         int                    `json:"year"`
	Court        string                 `json:"court"`
	Jurisdiction string                 `json:"jurisdiction"`
	
	// Legal principles
	Principles   []string               `json:"principles"`
	Holdings     []string               `json:"holdings"`
	Keywords     []string               `json:"keywords"`
	
	// Case details
	Facts        string                 `json:"facts"`
	Issue        string                 `json:"issue"`
	Holding      string                 `json:"holding"`
	Reasoning    string                 `json:"reasoning"`
	
	// Impact and relevance
	Impact       string                 `json:"impact"` // "landmark", "significant", "moderate", "limited"
	Overruled    bool                   `json:"overruled"`
	Distinguished []string              `json:"distinguished"` // Cases that distinguished this one
	Followed     []string               `json:"followed"`      // Cases that followed this one
	
	// Categories
	Categories   []string               `json:"categories"`
	
	// Metadata
	Metadata     map[string]interface{} `json:"metadata"`
}

// NewPrecedentDatabase creates a new precedent database
func NewPrecedentDatabase() *PrecedentDatabase {
	db := &PrecedentDatabase{
		precedents: make(map[string]*Precedent),
		indexes:    make(map[string][]string),
	}
	
	// Initialize with key precedents
	db.initializePrecedents()
	
	return db
}

// FindRelevantPrecedents finds precedents relevant to a test case
func (db *PrecedentDatabase) FindRelevantPrecedents(testCase *TestCase) []*Precedent {
	relevantPrecedents := make([]*Precedent, 0)
	scores := make(map[string]float64)
	
	// Score each precedent for relevance
	for id, precedent := range db.precedents {
		score := precedent.CalculateRelevance(testCase)
		if score > 0.3 { // Relevance threshold
			scores[id] = score
		}
	}
	
	// Sort by relevance score
	type scoredPrecedent struct {
		id    string
		score float64
	}
	
	var sorted []scoredPrecedent
	for id, score := range scores {
		sorted = append(sorted, scoredPrecedent{id, score})
	}
	
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].score > sorted[j].score
	})
	
	// Return top relevant precedents
	maxResults := 10
	for i, sp := range sorted {
		if i >= maxResults {
			break
		}
		relevantPrecedents = append(relevantPrecedents, db.precedents[sp.id])
	}
	
	return relevantPrecedents
}

// CalculateRelevance calculates how relevant this precedent is to a test case
func (p *Precedent) CalculateRelevance(testCase *TestCase) float64 {
	relevance := 0.0
	
	// Jurisdiction match
	if p.Jurisdiction == testCase.Jurisdiction {
		relevance += 0.2
	}
	
	// Category match
	categoryMatches := 0
	for _, cat := range p.Categories {
		if cat == testCase.Type {
			categoryMatches++
		}
		if testCase.Type == "dmca" && strings.Contains(cat, "copyright") {
			categoryMatches++
		}
		if testCase.Type == "privacy" && strings.Contains(cat, "privacy") {
			categoryMatches++
		}
	}
	relevance += float64(categoryMatches) * 0.15
	
	// Keyword matches
	keywordMatches := 0
	testKeywords := extractKeywords(testCase)
	for _, keyword := range p.Keywords {
		for _, testKeyword := range testKeywords {
			if strings.Contains(strings.ToLower(keyword), strings.ToLower(testKeyword)) {
				keywordMatches++
			}
		}
	}
	relevance += math.Min(float64(keywordMatches)*0.05, 0.3)
	
	// Specific relevance for DMCA cases
	if testCase.Type == "dmca" {
		if strings.Contains(p.Name, "Sony") && strings.Contains(p.Name, "Universal") {
			relevance += 0.3 // Betamax case highly relevant
		}
		if strings.Contains(p.Name, "Feist") {
			relevance += 0.25 // Originality requirement
		}
		if strings.Contains(p.Name, "Viacom") && strings.Contains(p.Name, "YouTube") {
			relevance += 0.25 // Safe harbor
		}
		if strings.Contains(p.Name, "Perfect 10") {
			relevance += 0.2 // Transformative use
		}
		
		// Check for specific principles
		for _, principle := range p.Principles {
			if strings.Contains(principle, "fair use") {
				relevance += 0.1
			}
			if strings.Contains(principle, "safe harbor") {
				relevance += 0.15
			}
			if strings.Contains(principle, "substantial non-infringing") {
				relevance += 0.2
			}
		}
	}
	
	// Recency bonus (more recent = more relevant)
	yearsSince := 2024 - p.Year
	if yearsSince < 5 {
		relevance += 0.1
	} else if yearsSince < 10 {
		relevance += 0.05
	}
	
	// Impact bonus
	switch p.Impact {
	case "landmark":
		relevance += 0.2
	case "significant":
		relevance += 0.1
	case "moderate":
		relevance += 0.05
	}
	
	// Penalty if overruled
	if p.Overruled {
		relevance *= 0.3
	}
	
	return math.Min(relevance, 1.0)
}

// IsFavorable determines if this precedent is favorable for the defendant
func (p *Precedent) IsFavorable(testCase *TestCase) bool {
	// For NoiseFS, we're generally defending
	favorable := false
	
	// Check holdings for favorable language
	for _, holding := range p.Holdings {
		holdingLower := strings.ToLower(holding)
		
		// Technology protection
		if strings.Contains(holdingLower, "substantial non-infringing uses") ||
		   strings.Contains(holdingLower, "capable of substantial non-infringing") {
			favorable = true
		}
		
		// Copyright limitations
		if strings.Contains(holdingLower, "not copyrightable") ||
		   strings.Contains(holdingLower, "lack of originality") ||
		   strings.Contains(holdingLower, "merger doctrine") {
			favorable = true
		}
		
		// Safe harbor
		if strings.Contains(holdingLower, "safe harbor") ||
		   strings.Contains(holdingLower, "not liable for user") {
			favorable = true
		}
		
		// Fair use
		if strings.Contains(holdingLower, "fair use") ||
		   strings.Contains(holdingLower, "transformative") {
			favorable = true
		}
		
		// Against broad copyright
		if strings.Contains(holdingLower, "copyright misuse") ||
		   strings.Contains(holdingLower, "first sale") {
			favorable = true
		}
	}
	
	// Check specific cases known to be favorable
	if strings.Contains(p.Name, "Sony") && strings.Contains(p.Name, "Universal") {
		favorable = true // Betamax
	}
	if strings.Contains(p.Name, "Feist") {
		favorable = true // No copyright in facts
	}
	
	return favorable
}

// ArgumentEvaluator evaluates the strength of legal arguments
type ArgumentEvaluator struct {
	precedentDB *PrecedentDatabase
	factors     map[string]float64
}

// NewArgumentEvaluator creates a new argument evaluator
func NewArgumentEvaluator() *ArgumentEvaluator {
	return &ArgumentEvaluator{
		precedentDB: NewPrecedentDatabase(),
		factors: map[string]float64{
			"precedent_support":    0.3,
			"logical_consistency":  0.2,
			"factual_support":      0.2,
			"policy_alignment":     0.15,
			"technical_accuracy":   0.15,
		},
	}
}

// EvaluateArgument evaluates the strength of a legal argument
func (ae *ArgumentEvaluator) EvaluateArgument(
	argument string,
	supportingFacts []string,
	opposingFacts []string,
	testCase *TestCase,
) *ArgumentEvaluation {
	
	eval := &ArgumentEvaluation{
		Argument:        argument,
		OverallStrength: 0.0,
		Factors:         make(map[string]float64),
		Weaknesses:      make([]string, 0),
		Strengths:       make([]string, 0),
	}
	
	// Evaluate precedent support
	precedentScore := ae.evaluatePrecedentSupport(argument, testCase)
	eval.Factors["precedent_support"] = precedentScore
	
	// Evaluate logical consistency
	logicalScore := ae.evaluateLogicalConsistency(argument, supportingFacts, opposingFacts)
	eval.Factors["logical_consistency"] = logicalScore
	
	// Evaluate factual support
	factualScore := ae.evaluateFactualSupport(argument, supportingFacts, opposingFacts, testCase)
	eval.Factors["factual_support"] = factualScore
	
	// Evaluate policy alignment
	policyScore := ae.evaluatePolicyAlignment(argument, testCase)
	eval.Factors["policy_alignment"] = policyScore
	
	// Evaluate technical accuracy
	technicalScore := ae.evaluateTechnicalAccuracy(argument, testCase)
	eval.Factors["technical_accuracy"] = technicalScore
	
	// Calculate overall strength
	for factor, weight := range ae.factors {
		eval.OverallStrength += eval.Factors[factor] * weight
	}
	
	// Identify strengths and weaknesses
	ae.identifyStrengthsWeaknesses(eval)
	
	return eval
}

// ArgumentEvaluation contains the evaluation results for an argument
type ArgumentEvaluation struct {
	Argument        string             `json:"argument"`
	OverallStrength float64            `json:"overall_strength"`
	Factors         map[string]float64 `json:"factors"`
	Weaknesses      []string           `json:"weaknesses"`
	Strengths       []string           `json:"strengths"`
	Precedents      []string           `json:"supporting_precedents"`
}

func (ae *ArgumentEvaluator) evaluatePrecedentSupport(argument string, testCase *TestCase) float64 {
	score := 0.0
	
	// Find precedents that support this argument
	relevantPrecedents := ae.precedentDB.FindRelevantPrecedents(testCase)
	supportingPrecedents := 0
	
	argumentLower := strings.ToLower(argument)
	
	for _, p := range relevantPrecedents {
		for _, holding := range p.Holdings {
			if ae.holdingSupportsArgument(holding, argumentLower) {
				supportingPrecedents++
				score += 0.2
			}
		}
		
		for _, principle := range p.Principles {
			if ae.principleSupportsArgument(principle, argumentLower) {
				supportingPrecedents++
				score += 0.1
			}
		}
	}
	
	// Cap the score
	return math.Min(score, 1.0)
}

func (ae *ArgumentEvaluator) holdingSupportsArgument(holding, argument string) bool {
	holdingLower := strings.ToLower(holding)
	
	// Check for conceptual alignment
	if strings.Contains(argument, "blocks cannot be copyrighted") {
		return strings.Contains(holdingLower, "lack") && strings.Contains(holdingLower, "originality") ||
		       strings.Contains(holdingLower, "not copyrightable")
	}
	
	if strings.Contains(argument, "public domain") {
		return strings.Contains(holdingLower, "public domain") ||
		       strings.Contains(holdingLower, "not protected")
	}
	
	if strings.Contains(argument, "safe harbor") {
		return strings.Contains(holdingLower, "safe harbor") ||
		       strings.Contains(holdingLower, "service provider") && strings.Contains(holdingLower, "not liable")
	}
	
	if strings.Contains(argument, "transformative") {
		return strings.Contains(holdingLower, "transformative") ||
		       strings.Contains(holdingLower, "fair use")
	}
	
	return false
}

func (ae *ArgumentEvaluator) principleSupportsArgument(principle, argument string) bool {
	principleLower := strings.ToLower(principle)
	
	// Similar logic to holding support
	return strings.Contains(principleLower, "technology") && strings.Contains(argument, "technology") ||
	       strings.Contains(principleLower, "copyright") && strings.Contains(argument, "copyright") ||
	       strings.Contains(principleLower, "fair use") && strings.Contains(argument, "fair use")
}

func (ae *ArgumentEvaluator) evaluateLogicalConsistency(
	argument string,
	supportingFacts []string,
	opposingFacts []string,
) float64 {
	
	score := 0.7 // Base score
	
	// Check if supporting facts actually support the argument
	relevantSupporting := 0
	for _, fact := range supportingFacts {
		if ae.factSupportsArgument(fact, argument) {
			relevantSupporting++
		}
	}
	
	if len(supportingFacts) > 0 {
		score += float64(relevantSupporting) / float64(len(supportingFacts)) * 0.2
	}
	
	// Check if opposing facts undermine the argument
	underminingFacts := 0
	for _, fact := range opposingFacts {
		if ae.factUnderminesArgument(fact, argument) {
			underminingFacts++
		}
	}
	
	if len(opposingFacts) > 0 {
		score -= float64(underminingFacts) / float64(len(opposingFacts)) * 0.3
	}
	
	return math.Max(0, math.Min(score, 1.0))
}

func (ae *ArgumentEvaluator) factSupportsArgument(fact, argument string) bool {
	factLower := strings.ToLower(fact)
	argumentLower := strings.ToLower(argument)
	
	// Simple keyword matching for demonstration
	keywords := extractKeywordsFromString(argumentLower)
	matches := 0
	
	for _, keyword := range keywords {
		if strings.Contains(factLower, keyword) {
			matches++
		}
	}
	
	return matches >= 2
}

func (ae *ArgumentEvaluator) factUnderminesArgument(fact, argument string) bool {
	// Simplified logic - in reality would be more sophisticated
	return strings.Contains(fact, "contrary to") ||
	       strings.Contains(fact, "disproves") ||
	       strings.Contains(fact, "inconsistent with")
}

func (ae *ArgumentEvaluator) evaluateFactualSupport(
	argument string,
	supportingFacts []string,
	opposingFacts []string,
	testCase *TestCase,
) float64 {
	
	score := 0.5 // Base score
	
	// Boost score for specific factual support in DMCA cases
	if testCase.Type == "dmca" && testCase.DMCADetails != nil {
		if strings.Contains(argument, "public domain") {
			score += testCase.DMCADetails.PublicDomainMix * 0.3
		}
		
		if strings.Contains(argument, "blocks") && strings.Contains(argument, "shared") {
			if testCase.DMCADetails.BlocksShared > 100 {
				score += 0.2
			}
		}
		
		if strings.Contains(argument, "encryption") && testCase.DMCADetails.EncryptionUsed {
			score += 0.1
		}
	}
	
	// Factor in the balance of supporting vs opposing facts
	if len(supportingFacts) > len(opposingFacts)*2 {
		score += 0.2
	} else if len(opposingFacts) > len(supportingFacts)*2 {
		score -= 0.2
	}
	
	return math.Max(0, math.Min(score, 1.0))
}

func (ae *ArgumentEvaluator) evaluatePolicyAlignment(argument string, testCase *TestCase) float64 {
	score := 0.5 // Base score
	
	argumentLower := strings.ToLower(argument)
	
	// Pro-innovation policy
	if strings.Contains(argumentLower, "innovation") ||
	   strings.Contains(argumentLower, "technology") ||
	   strings.Contains(argumentLower, "progress") {
		score += 0.2
	}
	
	// Privacy protection policy
	if strings.Contains(argumentLower, "privacy") ||
	   strings.Contains(argumentLower, "anonymity") ||
	   strings.Contains(argumentLower, "security") {
		score += 0.15
	}
	
	// Balance of interests
	if strings.Contains(argumentLower, "balance") ||
	   strings.Contains(argumentLower, "legitimate") ||
	   strings.Contains(argumentLower, "reasonable") {
		score += 0.1
	}
	
	// Free speech considerations
	if strings.Contains(argumentLower, "speech") ||
	   strings.Contains(argumentLower, "expression") ||
	   strings.Contains(argumentLower, "communication") {
		score += 0.1
	}
	
	return math.Min(score, 1.0)
}

func (ae *ArgumentEvaluator) evaluateTechnicalAccuracy(argument string, testCase *TestCase) float64 {
	score := 0.8 // Assume technical arguments are generally accurate
	
	argumentLower := strings.ToLower(argument)
	
	// Check for technical concepts
	if strings.Contains(argumentLower, "xor") ||
	   strings.Contains(argumentLower, "encryption") ||
	   strings.Contains(argumentLower, "hash") {
		score += 0.1 // Technical sophistication bonus
	}
	
	// Verify technical claims match case facts
	if testCase.Type == "dmca" && testCase.DMCADetails != nil {
		if strings.Contains(argumentLower, "encrypted") && !testCase.DMCADetails.EncryptionUsed {
			score -= 0.3 // Inaccurate claim
		}
		
		if strings.Contains(argumentLower, "descriptor") && !testCase.DMCADetails.DescriptorOnly {
			score -= 0.2 // Misrepresentation
		}
	}
	
	return math.Max(0, math.Min(score, 1.0))
}

func (ae *ArgumentEvaluator) identifyStrengthsWeaknesses(eval *ArgumentEvaluation) {
	// Identify strengths
	for factor, score := range eval.Factors {
		if score > 0.7 {
			switch factor {
			case "precedent_support":
				eval.Strengths = append(eval.Strengths, "Strong precedent support")
			case "logical_consistency":
				eval.Strengths = append(eval.Strengths, "Logically consistent argument")
			case "factual_support":
				eval.Strengths = append(eval.Strengths, "Well-supported by facts")
			case "policy_alignment":
				eval.Strengths = append(eval.Strengths, "Aligns with public policy")
			case "technical_accuracy":
				eval.Strengths = append(eval.Strengths, "Technically accurate")
			}
		}
	}
	
	// Identify weaknesses
	for factor, score := range eval.Factors {
		if score < 0.4 {
			switch factor {
			case "precedent_support":
				eval.Weaknesses = append(eval.Weaknesses, "Limited precedent support")
			case "logical_consistency":
				eval.Weaknesses = append(eval.Weaknesses, "Logical inconsistencies")
			case "factual_support":
				eval.Weaknesses = append(eval.Weaknesses, "Weak factual foundation")
			case "policy_alignment":
				eval.Weaknesses = append(eval.Weaknesses, "Policy concerns")
			case "technical_accuracy":
				eval.Weaknesses = append(eval.Weaknesses, "Technical inaccuracies")
			}
		}
	}
}

// OutcomePredictor predicts case outcomes based on various factors
type OutcomePredictor struct {
	historicalData map[string]*HistoricalOutcome
	mlModel        *SimpleMLModel // Simplified ML model
}

// HistoricalOutcome represents historical case outcome data
type HistoricalOutcome struct {
	CaseType       string
	Jurisdiction   string
	YearRange      [2]int
	DefendantWinRate float64
	SettlementRate   float64
	AppealRate       float64
	AverageDuration  int // days
	AverageCost      float64
}

// SimpleMLModel is a simplified ML model for predictions
type SimpleMLModel struct {
	weights map[string]float64
}

// NewOutcomePredictor creates a new outcome predictor
func NewOutcomePredictor() *OutcomePredictor {
	predictor := &OutcomePredictor{
		historicalData: make(map[string]*HistoricalOutcome),
		mlModel: &SimpleMLModel{
			weights: map[string]float64{
				"precedent_strength":    0.25,
				"argument_quality":      0.20,
				"factual_support":       0.20,
				"jurisdiction_bias":     0.15,
				"case_complexity":       0.10,
				"public_interest":       0.10,
			},
		},
	}
	
	// Initialize with historical data
	predictor.initializeHistoricalData()
	
	return predictor
}

func (op *OutcomePredictor) initializeHistoricalData() {
	// DMCA cases in US
	op.historicalData["dmca_us"] = &HistoricalOutcome{
		CaseType:         "dmca",
		Jurisdiction:     "US",
		YearRange:        [2]int{2010, 2023},
		DefendantWinRate: 0.35, // Service providers win ~35% of DMCA cases
		SettlementRate:   0.40, // 40% settle
		AppealRate:       0.25, // 25% are appealed
		AverageDuration:  450,  // ~15 months
		AverageCost:      250000,
	}
	
	// Privacy cases in US
	op.historicalData["privacy_us"] = &HistoricalOutcome{
		CaseType:         "privacy",
		Jurisdiction:     "US",
		YearRange:        [2]int{2010, 2023},
		DefendantWinRate: 0.45,
		SettlementRate:   0.35,
		AppealRate:       0.20,
		AverageDuration:  365,
		AverageCost:      150000,
	}
	
	// GDPR cases in EU
	op.historicalData["privacy_eu"] = &HistoricalOutcome{
		CaseType:         "privacy",
		Jurisdiction:     "EU",
		YearRange:        [2]int{2018, 2023},
		DefendantWinRate: 0.30, // GDPR is strict
		SettlementRate:   0.45,
		AppealRate:       0.15,
		AverageDuration:  300,
		AverageCost:      200000,
	}
}

// PredictOutcome predicts the outcome of a case
func (op *OutcomePredictor) PredictOutcome(
	testCase *TestCase,
	argumentStrengths map[string]float64,
	precedentSupport float64,
) *OutcomePrediction {
	
	prediction := &OutcomePrediction{
		Probabilities: make(map[string]float64),
		Confidence:    0.0,
		Factors:       make(map[string]float64),
	}
	
	// Get historical baseline
	historical := op.getHistoricalBaseline(testCase)
	
	// Calculate feature scores
	features := op.extractFeatures(testCase, argumentStrengths, precedentSupport)
	
	// Apply ML model
	defendantScore := op.mlModel.predict(features)
	
	// Adjust based on historical data
	if historical != nil {
		defendantScore = defendantScore*0.7 + historical.DefendantWinRate*0.3
		prediction.Probabilities["settlement"] = historical.SettlementRate
	}
	
	// Set probabilities
	prediction.Probabilities["defendant_wins"] = defendantScore
	prediction.Probabilities["plaintiff_wins"] = 1.0 - defendantScore - prediction.Probabilities["settlement"]
	
	// Calculate confidence based on feature strength
	prediction.Confidence = op.calculateConfidence(features)
	
	// Store factors for explainability
	prediction.Factors = features
	
	// Predict most likely outcome
	prediction.MostLikely = op.selectMostLikely(prediction.Probabilities)
	
	// Additional predictions
	if historical != nil {
		prediction.EstimatedDuration = historical.AverageDuration
		prediction.EstimatedCost = historical.AverageCost
		prediction.AppealLikelihood = historical.AppealRate
	}
	
	return prediction
}

// OutcomePrediction contains predicted case outcome
type OutcomePrediction struct {
	MostLikely        string               `json:"most_likely"`
	Probabilities     map[string]float64   `json:"probabilities"`
	Confidence        float64              `json:"confidence"`
	EstimatedDuration int                  `json:"estimated_duration_days"`
	EstimatedCost     float64              `json:"estimated_cost"`
	AppealLikelihood  float64              `json:"appeal_likelihood"`
	Factors           map[string]float64   `json:"factors"`
}

func (op *OutcomePredictor) getHistoricalBaseline(testCase *TestCase) *HistoricalOutcome {
	key := fmt.Sprintf("%s_%s", testCase.Type, testCase.Jurisdiction)
	key = strings.ToLower(key)
	return op.historicalData[key]
}

func (op *OutcomePredictor) extractFeatures(
	testCase *TestCase,
	argumentStrengths map[string]float64,
	precedentSupport float64,
) map[string]float64 {
	
	features := make(map[string]float64)
	
	// Precedent support
	features["precedent_strength"] = precedentSupport
	
	// Argument quality (average of all argument strengths)
	if len(argumentStrengths) > 0 {
		sum := 0.0
		for _, strength := range argumentStrengths {
			sum += strength
		}
		features["argument_quality"] = sum / float64(len(argumentStrengths))
	}
	
	// Factual support (based on case specifics)
	features["factual_support"] = op.assessFactualSupport(testCase)
	
	// Jurisdiction bias
	features["jurisdiction_bias"] = op.assessJurisdictionBias(testCase)
	
	// Case complexity
	features["case_complexity"] = op.assessComplexity(testCase)
	
	// Public interest
	features["public_interest"] = op.assessPublicInterest(testCase)
	
	return features
}

func (op *OutcomePredictor) assessFactualSupport(testCase *TestCase) float64 {
	score := 0.5
	
	if testCase.Type == "dmca" && testCase.DMCADetails != nil {
		// Strong facts for defense
		if testCase.DMCADetails.PublicDomainMix > 0.5 {
			score += 0.2
		}
		if testCase.DMCADetails.BlocksShared > 100 {
			score += 0.15
		}
		if testCase.DMCADetails.DescriptorOnly {
			score += 0.1
		}
		
		// Weak facts for defense
		if testCase.DMCADetails.ClaimValidity == "valid" {
			score -= 0.15
		}
		if testCase.DMCADetails.ClaimedDamages > 500000 {
			score -= 0.1
		}
	}
	
	return math.Max(0, math.Min(score, 1.0))
}

func (op *OutcomePredictor) assessJurisdictionBias(testCase *TestCase) float64 {
	// Different jurisdictions have different biases
	switch testCase.Jurisdiction {
	case "US":
		if testCase.Type == "dmca" {
			return 0.5 // Neutral
		} else if testCase.Type == "privacy" {
			return 0.4 // Slightly pro-business
		}
	case "EU":
		if testCase.Type == "privacy" {
			return 0.3 // Pro-privacy
		}
	}
	
	return 0.5 // Default neutral
}

func (op *OutcomePredictor) assessComplexity(testCase *TestCase) float64 {
	complexity := 0.3 // Base complexity
	
	// Technical cases are more complex
	if testCase.Type == "dmca" && testCase.DMCADetails != nil {
		if testCase.DMCADetails.EncryptionUsed {
			complexity += 0.2
		}
		if testCase.DMCADetails.BlocksShared > 500 {
			complexity += 0.1
		}
	}
	
	// Multiple legal theories increase complexity
	if len(testCase.Challenges) > 3 {
		complexity += 0.2
	}
	
	return math.Min(complexity, 1.0)
}

func (op *OutcomePredictor) assessPublicInterest(testCase *TestCase) float64 {
	interest := 0.3 // Base interest
	
	// Privacy cases have high public interest
	if testCase.Type == "privacy" {
		interest += 0.3
	}
	
	// Large damage claims attract interest
	if testCase.Type == "dmca" && testCase.DMCADetails != nil {
		if testCase.DMCADetails.ClaimedDamages > 1000000 {
			interest += 0.2
		}
	}
	
	// Novel legal issues
	if testCase.RiskLevel == "high" {
		interest += 0.2
	}
	
	return math.Min(interest, 1.0)
}

func (mlm *SimpleMLModel) predict(features map[string]float64) float64 {
	score := 0.0
	
	for feature, value := range features {
		if weight, exists := mlm.weights[feature]; exists {
			score += value * weight
		}
	}
	
	// Sigmoid activation to get probability
	return 1.0 / (1.0 + math.Exp(-score))
}

func (op *OutcomePredictor) calculateConfidence(features map[string]float64) float64 {
	// Confidence based on feature strength and consistency
	var values []float64
	for _, v := range features {
		values = append(values, v)
	}
	
	// Calculate standard deviation
	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))
	
	variance := 0.0
	for _, v := range values {
		variance += math.Pow(v-mean, 2)
	}
	variance /= float64(len(values))
	stdDev := math.Sqrt(variance)
	
	// Lower std dev = higher confidence (more consistent features)
	confidence := 1.0 - stdDev
	
	// Adjust based on extreme values
	for _, v := range values {
		if v > 0.9 || v < 0.1 {
			confidence += 0.05 // Extreme values increase confidence
		}
	}
	
	return math.Max(0.3, math.Min(confidence, 0.95))
}

func (op *OutcomePredictor) selectMostLikely(probabilities map[string]float64) string {
	maxProb := 0.0
	mostLikely := ""
	
	for outcome, prob := range probabilities {
		if prob > maxProb {
			maxProb = prob
			mostLikely = outcome
		}
	}
	
	return mostLikely
}

// Initialize precedent database with key cases
func (db *PrecedentDatabase) initializePrecedents() {
	// Sony v. Universal (Betamax case)
	db.addPrecedent(&Precedent{
		ID:           "sony_universal_1984",
		Name:         "Sony Corp. of America v. Universal City Studios, Inc.",
		Citation:     "464 U.S. 417 (1984)",
		Year:         1984,
		Court:        "Supreme Court",
		Jurisdiction: "US",
		Principles: []string{
			"Substantial non-infringing uses",
			"Technology protection",
			"Fair use for time-shifting",
		},
		Holdings: []string{
			"Technology capable of substantial non-infringing uses is not liable for contributory infringement",
			"Time-shifting television programs is fair use",
		},
		Keywords: []string{"betamax", "vcr", "technology", "substantial", "non-infringing", "fair use"},
		Facts:    "Sony manufactured Betamax VCRs that could record television programs",
		Issue:    "Whether manufacturers of technology that can be used for copyright infringement are liable",
		Holding:  "Technology with substantial non-infringing uses is protected",
		Impact:   "landmark",
		Categories: []string{"copyright", "technology", "secondary_liability"},
	})
	
	// Feist v. Rural Telephone
	db.addPrecedent(&Precedent{
		ID:           "feist_rural_1991",
		Name:         "Feist Publications, Inc. v. Rural Telephone Service Co.",
		Citation:     "499 U.S. 340 (1991)",
		Year:         1991,
		Court:        "Supreme Court",
		Jurisdiction: "US",
		Principles: []string{
			"Originality requirement for copyright",
			"No copyright in facts",
			"Minimal creativity required",
		},
		Holdings: []string{
			"Facts are not copyrightable",
			"Compilations of facts require minimal creativity to be protected",
			"Sweat of the brow alone is insufficient for copyright",
		},
		Keywords: []string{"originality", "facts", "compilation", "creativity", "phone book"},
		Facts:    "Rural Telephone created white pages directory, Feist copied listings",
		Issue:    "Whether factual compilations without creativity are copyrightable",
		Holding:  "Copyright requires originality and minimal creativity",
		Impact:   "landmark",
		Categories: []string{"copyright", "originality", "compilation"},
	})
	
	// Viacom v. YouTube
	db.addPrecedent(&Precedent{
		ID:           "viacom_youtube_2012",
		Name:         "Viacom International Inc. v. YouTube, Inc.",
		Citation:     "676 F.3d 19 (2d Cir. 2012)",
		Year:         2012,
		Court:        "Second Circuit",
		Jurisdiction: "US",
		Principles: []string{
			"DMCA safe harbor for service providers",
			"Knowledge standard for liability",
			"Red flag knowledge",
		},
		Holdings: []string{
			"Service providers protected by safe harbor if they lack specific knowledge",
			"General awareness of infringement insufficient for liability",
			"Must have knowledge of specific infringing activity",
		},
		Keywords: []string{"dmca", "safe harbor", "youtube", "service provider", "knowledge"},
		Facts:    "Viacom sued YouTube for hosting infringing videos",
		Issue:    "Scope of DMCA safe harbor protection",
		Holding:  "Safe harbor requires specific knowledge of infringement",
		Impact:   "significant",
		Categories: []string{"dmca", "copyright", "safe_harbor", "internet"},
	})
	
	// Perfect 10 v. Amazon
	db.addPrecedent(&Precedent{
		ID:           "perfect10_amazon_2007",
		Name:         "Perfect 10, Inc. v. Amazon.com, Inc.",
		Citation:     "508 F.3d 1146 (9th Cir. 2007)",
		Year:         2007,
		Court:        "Ninth Circuit",
		Jurisdiction: "US",
		Principles: []string{
			"Transformative use in fair use analysis",
			"Server test for direct infringement",
			"Inline linking not infringement",
		},
		Holdings: []string{
			"Transformative use weighs in favor of fair use",
			"Search engine thumbnails can be fair use",
			"Inline linking does not constitute direct infringement",
		},
		Keywords: []string{"transformative", "fair use", "search engine", "thumbnails", "linking"},
		Facts:    "Perfect 10 sued Amazon/Google for displaying thumbnail images",
		Issue:    "Whether search engine image results infringe copyright",
		Holding:  "Transformative purpose can support fair use",
		Impact:   "significant",
		Categories: []string{"copyright", "fair_use", "internet", "search"},
	})
	
	// Authors Guild v. Google
	db.addPrecedent(&Precedent{
		ID:           "authors_guild_google_2015",
		Name:         "Authors Guild v. Google, Inc.",
		Citation:     "804 F.3d 202 (2d Cir. 2015)",
		Year:         2015,
		Court:        "Second Circuit",
		Jurisdiction: "US",
		Principles: []string{
			"Transformative use for book scanning",
			"Public benefit in fair use",
			"Digital preservation",
		},
		Holdings: []string{
			"Mass digitization for search is transformative fair use",
			"Snippet view does not harm market for books",
			"Public benefit weighs in favor of fair use",
		},
		Keywords: []string{"books", "scanning", "search", "transformative", "snippets", "library"},
		Facts:    "Google scanned millions of books for search database",
		Issue:    "Whether mass digitization of books is fair use",
		Holding:  "Transformative purpose and public benefit support fair use",
		Impact:   "significant",
		Categories: []string{"copyright", "fair_use", "digitization", "search"},
	})
	
	// Campbell v. Acuff-Rose (2 Live Crew)
	db.addPrecedent(&Precedent{
		ID:           "campbell_acuff_rose_1994",
		Name:         "Campbell v. Acuff-Rose Music, Inc.",
		Citation:     "510 U.S. 569 (1994)",
		Year:         1994,
		Court:        "Supreme Court",
		Jurisdiction: "US",
		Principles: []string{
			"Parody as fair use",
			"Commercial nature not dispositive",
			"Transformative use analysis",
		},
		Holdings: []string{
			"Parody can be fair use even if commercial",
			"Transformative works less likely to harm market",
			"Must consider all four fair use factors",
		},
		Keywords: []string{"parody", "fair use", "transformative", "commercial", "rap"},
		Facts:    "2 Live Crew created rap parody of 'Oh, Pretty Woman'",
		Issue:    "Whether commercial parody can be fair use",
		Holding:  "Commercial parody can qualify as fair use",
		Impact:   "landmark",
		Categories: []string{"copyright", "fair_use", "parody", "music"},
	})
	
	// MGM v. Grokster
	db.addPrecedent(&Precedent{
		ID:           "mgm_grokster_2005",
		Name:         "MGM Studios Inc. v. Grokster, Ltd.",
		Citation:     "545 U.S. 913 (2005)",
		Year:         2005,
		Court:        "Supreme Court",
		Jurisdiction: "US",
		Principles: []string{
			"Inducement liability",
			"Intent to induce infringement",
			"Technology and copyright balance",
		},
		Holdings: []string{
			"Actively inducing infringement creates liability",
			"Sony defense unavailable when inducement shown",
			"Evidence of unlawful intent defeats technology protection",
		},
		Keywords: []string{"p2p", "file sharing", "inducement", "grokster", "technology"},
		Facts:    "P2P file sharing services used primarily for infringement",
		Issue:    "Liability for distributing file-sharing software",
		Holding:  "Active inducement of infringement creates liability",
		Impact:   "significant",
		Categories: []string{"copyright", "technology", "secondary_liability", "p2p"},
	})
	
	// Lenz v. Universal (Dancing Baby)
	db.addPrecedent(&Precedent{
		ID:           "lenz_universal_2015",
		Name:         "Lenz v. Universal Music Corp.",
		Citation:     "815 F.3d 1145 (9th Cir. 2015)",
		Year:         2015,
		Court:        "Ninth Circuit",
		Jurisdiction: "US",
		Principles: []string{
			"Fair use consideration in DMCA notices",
			"Good faith requirement",
			"Section 512(f) liability",
		},
		Holdings: []string{
			"Copyright holders must consider fair use before sending takedown",
			"Failure to consider fair use may violate 512(f)",
			"Subjective good faith belief required",
		},
		Keywords: []string{"dmca", "takedown", "fair use", "512f", "dancing baby"},
		Facts:    "Universal sent takedown for home video with background music",
		Issue:    "Whether fair use must be considered before DMCA takedown",
		Holding:  "Fair use consideration required for valid takedown",
		Impact:   "significant",
		Categories: []string{"dmca", "fair_use", "takedown", "internet"},
	})
	
	// Capitol Records v. MP3tunes
	db.addPrecedent(&Precedent{
		ID:           "capitol_mp3tunes_2013",
		Name:         "Capitol Records, LLC v. MP3tunes, LLC",
		Citation:     "821 F. Supp. 2d 627 (S.D.N.Y. 2013)",
		Year:         2013,
		Court:        "Southern District of New York",
		Jurisdiction: "US",
		Principles: []string{
			"Cloud storage safe harbor",
			"Red flag knowledge standard",
			"Repeat infringer policy",
		},
		Holdings: []string{
			"Cloud storage services can qualify for safe harbor",
			"Must reasonably implement repeat infringer policy",
			"Willful blindness incompatible with safe harbor",
		},
		Keywords: []string{"cloud", "storage", "dmca", "safe harbor", "repeat infringer"},
		Facts:    "MP3tunes operated cloud music storage service",
		Issue:    "DMCA safe harbor for cloud storage services",
		Holding:  "Cloud storage eligible for safe harbor with proper policies",
		Impact:   "moderate",
		Categories: []string{"dmca", "cloud", "storage", "safe_harbor"},
	})
}

func (db *PrecedentDatabase) addPrecedent(precedent *Precedent) {
	db.precedents[precedent.ID] = precedent
	
	// Build indexes
	// By year
	yearKey := fmt.Sprintf("year_%d", precedent.Year)
	db.indexes[yearKey] = append(db.indexes[yearKey], precedent.ID)
	
	// By court
	courtKey := fmt.Sprintf("court_%s", strings.ToLower(precedent.Court))
	db.indexes[courtKey] = append(db.indexes[courtKey], precedent.ID)
	
	// By category
	for _, cat := range precedent.Categories {
		catKey := fmt.Sprintf("cat_%s", cat)
		db.indexes[catKey] = append(db.indexes[catKey], precedent.ID)
	}
	
	// By keywords
	for _, keyword := range precedent.Keywords {
		keywordKey := fmt.Sprintf("kw_%s", strings.ToLower(keyword))
		db.indexes[keywordKey] = append(db.indexes[keywordKey], precedent.ID)
	}
}

// Utility functions

func extractKeywords(testCase *TestCase) []string {
	keywords := []string{testCase.Type, testCase.Scenario}
	
	if testCase.Type == "dmca" && testCase.DMCADetails != nil {
		keywords = append(keywords, 
			testCase.DMCADetails.ContentType,
			"copyright",
			"infringement",
		)
		
		if testCase.DMCADetails.EncryptionUsed {
			keywords = append(keywords, "encryption")
		}
		
		if testCase.DMCADetails.PublicDomainMix > 0.5 {
			keywords = append(keywords, "public domain")
		}
	}
	
	// Extract from description
	descKeywords := extractKeywordsFromString(testCase.Description)
	keywords = append(keywords, descKeywords...)
	
	return keywords
}

func extractKeywordsFromString(text string) []string {
	// Simple keyword extraction - in production would use NLP
	importantWords := []string{
		"copyright", "dmca", "infringement", "technology", "encryption",
		"privacy", "anonymous", "block", "descriptor", "public domain",
		"fair use", "safe harbor", "transformative", "substantial",
	}
	
	textLower := strings.ToLower(text)
	var found []string
	
	for _, word := range importantWords {
		if strings.Contains(textLower, word) {
			found = append(found, word)
		}
	}
	
	return found
}