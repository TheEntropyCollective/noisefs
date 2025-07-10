package cache

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// AccessPredictor implements ML-based access pattern prediction
type AccessPredictor struct {
	// Simple linear regression model for access prediction
	model           *LinearRegressionModel
	
	// Feature extractors
	featureExtractors []FeatureExtractor
	
	// Training data
	trainingData    []*TrainingExample
	lastTraining    time.Time
	trainingInterval time.Duration
	
	// Prediction cache
	predictionCache map[string]*PredictionResult
	cacheExpiry     time.Duration
}

// LinearRegressionModel implements a simple linear regression for access prediction
type LinearRegressionModel struct {
	weights         []float64
	bias            float64
	learningRate    float64
	trained         bool
}

// FeatureExtractor extracts features from access patterns
type FeatureExtractor interface {
	ExtractFeatures(pattern *AdaptiveAccessPattern, metadata map[string]interface{}) []float64
	GetFeatureNames() []string
}

// TrainingExample represents a training example for the ML model
type TrainingExample struct {
	Features        []float64
	Target          float64  // 1.0 if accessed within prediction window, 0.0 otherwise
	Timestamp       time.Time
}

// PredictionResult contains prediction results
type PredictionResult struct {
	CID             string
	Score           float64
	Confidence      float64
	PredictedTime   time.Time
	CreatedAt       time.Time
}

// NewAccessPredictor creates a new access predictor
func NewAccessPredictor() *AccessPredictor {
	predictor := &AccessPredictor{
		model:            NewLinearRegressionModel(0.01), // 1% learning rate
		featureExtractors: []FeatureExtractor{
			NewTemporalFeatureExtractor(),
			NewFrequencyFeatureExtractor(),
			NewRecencyFeatureExtractor(),
			NewMetadataFeatureExtractor(),
		},
		trainingData:     make([]*TrainingExample, 0),
		trainingInterval: time.Hour * 6, // Retrain every 6 hours
		predictionCache:  make(map[string]*PredictionResult),
		cacheExpiry:      time.Minute * 30, // Cache predictions for 30 minutes
	}
	
	return predictor
}

// PredictAccess predicts the likelihood of a block being accessed
func (ap *AccessPredictor) PredictAccess(cid string, metadata map[string]interface{}) float64 {
	// Check prediction cache first
	if cached, exists := ap.predictionCache[cid]; exists {
		if time.Since(cached.CreatedAt) < ap.cacheExpiry {
			return cached.Score
		}
		delete(ap.predictionCache, cid)
	}
	
	// Create a dummy access pattern for new blocks
	pattern := &AdaptiveAccessPattern{
		CID:         cid,
		AccessTimes: make([]time.Time, 0),
	}
	
	// Extract features
	features := ap.extractAllFeatures(pattern, metadata)
	
	// Make prediction
	score := ap.model.Predict(features)
	
	// Cache the prediction
	result := &PredictionResult{
		CID:       cid,
		Score:     score,
		CreatedAt: time.Now(),
	}
	ap.predictionCache[cid] = result
	
	return score
}

// PredictNextAccess predicts when a block will be accessed next
func (ap *AccessPredictor) PredictNextAccess(pattern *AdaptiveAccessPattern) float64 {
	if len(pattern.AccessTimes) == 0 {
		return 0.0
	}
	
	// Extract features from access pattern
	features := ap.extractAllFeatures(pattern, nil)
	
	// Make prediction
	score := ap.model.Predict(features)
	
	// Apply temporal adjustments
	score = ap.applyTemporalAdjustments(score, pattern)
	
	return score
}

// extractAllFeatures extracts features using all feature extractors
func (ap *AccessPredictor) extractAllFeatures(pattern *AdaptiveAccessPattern, metadata map[string]interface{}) []float64 {
	var allFeatures []float64
	
	for _, extractor := range ap.featureExtractors {
		features := extractor.ExtractFeatures(pattern, metadata)
		allFeatures = append(allFeatures, features...)
	}
	
	return allFeatures
}

// applyTemporalAdjustments applies time-based adjustments to predictions
func (ap *AccessPredictor) applyTemporalAdjustments(score float64, pattern *AdaptiveAccessPattern) float64 {
	if len(pattern.AccessTimes) == 0 {
		return score
	}
	
	now := time.Now()
	lastAccess := pattern.AccessTimes[len(pattern.AccessTimes)-1]
	timeSinceLastAccess := now.Sub(lastAccess)
	
	// Decay factor based on time since last access
	decayFactor := math.Exp(-timeSinceLastAccess.Hours() / 24.0) // 24-hour half-life
	
	// Circadian rhythm adjustment
	currentHour := now.Hour()
	if pattern.DailyPattern[currentHour] > 0 {
		totalAccesses := 0
		for _, count := range pattern.DailyPattern {
			totalAccesses += count
		}
		hourlyProbability := float64(pattern.DailyPattern[currentHour]) / float64(totalAccesses)
		score *= (1.0 + hourlyProbability) // Boost if this hour has high activity
	}
	
	return score * decayFactor
}

// Train trains the ML model with collected data
func (ap *AccessPredictor) Train() error {
	if len(ap.trainingData) < 10 {
		return nil // Need at least 10 examples
	}
	
	// Prepare training data
	X := make([][]float64, len(ap.trainingData))
	y := make([]float64, len(ap.trainingData))
	
	for i, example := range ap.trainingData {
		X[i] = example.Features
		y[i] = example.Target
	}
	
	// Train the model
	err := ap.model.Train(X, y)
	if err != nil {
		return err
	}
	
	ap.lastTraining = time.Now()
	return nil
}

// AddTrainingExample adds a new training example
func (ap *AccessPredictor) AddTrainingExample(pattern *AdaptiveAccessPattern, wasAccessed bool, metadata map[string]interface{}) {
	features := ap.extractAllFeatures(pattern, metadata)
	
	target := 0.0
	if wasAccessed {
		target = 1.0
	}
	
	example := &TrainingExample{
		Features:  features,
		Target:    target,
		Timestamp: time.Now(),
	}
	
	ap.trainingData = append(ap.trainingData, example)
	
	// Limit training data size
	if len(ap.trainingData) > 10000 {
		ap.trainingData = ap.trainingData[1000:] // Keep recent 9000 examples
	}
	
	// Check if we should retrain
	if time.Since(ap.lastTraining) > ap.trainingInterval {
		go ap.Train() // Train in background
	}
}

// GetTopPredictions returns top predicted blocks
func (ap *AccessPredictor) GetTopPredictions(count int) []*PredictionResult {
	predictions := make([]*PredictionResult, 0, len(ap.predictionCache))
	
	for _, prediction := range ap.predictionCache {
		if time.Since(prediction.CreatedAt) < ap.cacheExpiry {
			predictions = append(predictions, prediction)
		}
	}
	
	// Sort by score (descending)
	sort.Slice(predictions, func(i, j int) bool {
		return predictions[i].Score > predictions[j].Score
	})
	
	if count > len(predictions) {
		count = len(predictions)
	}
	
	return predictions[:count]
}

// NewLinearRegressionModel creates a new linear regression model
func NewLinearRegressionModel(learningRate float64) *LinearRegressionModel {
	return &LinearRegressionModel{
		learningRate: learningRate,
		trained:      false,
	}
}

// Train trains the linear regression model
func (lrm *LinearRegressionModel) Train(X [][]float64, y []float64) error {
	if len(X) == 0 || len(X) != len(y) {
		return fmt.Errorf("invalid training data")
	}
	
	numFeatures := len(X[0])
	
	// Initialize weights if needed
	if len(lrm.weights) != numFeatures {
		lrm.weights = make([]float64, numFeatures)
		for i := range lrm.weights {
			lrm.weights[i] = 0.01 // Small random initialization
		}
	}
	
	// Gradient descent training
	epochs := 1000
	for epoch := 0; epoch < epochs; epoch++ {
		// Calculate gradients
		weightGradients := make([]float64, numFeatures)
		biasGradient := 0.0
		
		for i := 0; i < len(X); i++ {
			prediction := lrm.predict(X[i])
			error := prediction - y[i]
			
			// Update gradients
			for j := 0; j < numFeatures; j++ {
				weightGradients[j] += error * X[i][j]
			}
			biasGradient += error
		}
		
		// Apply gradients
		for j := 0; j < numFeatures; j++ {
			lrm.weights[j] -= lrm.learningRate * weightGradients[j] / float64(len(X))
		}
		lrm.bias -= lrm.learningRate * biasGradient / float64(len(X))
	}
	
	lrm.trained = true
	return nil
}

// Predict makes a prediction using the trained model
func (lrm *LinearRegressionModel) Predict(features []float64) float64 {
	if !lrm.trained || len(features) != len(lrm.weights) {
		return 0.5 // Default prediction
	}
	
	return lrm.predict(features)
}

// predict internal prediction function
func (lrm *LinearRegressionModel) predict(features []float64) float64 {
	prediction := lrm.bias
	for i, feature := range features {
		prediction += lrm.weights[i] * feature
	}
	
	// Apply sigmoid activation to get probability
	return 1.0 / (1.0 + math.Exp(-prediction))
}

// TemporalFeatureExtractor extracts time-based features
type TemporalFeatureExtractor struct{}

func NewTemporalFeatureExtractor() *TemporalFeatureExtractor {
	return &TemporalFeatureExtractor{}
}

func (tfe *TemporalFeatureExtractor) ExtractFeatures(pattern *AdaptiveAccessPattern, metadata map[string]interface{}) []float64 {
	now := time.Now()
	
	features := make([]float64, 4)
	
	// Time of day (normalized 0-1)
	features[0] = float64(now.Hour()) / 24.0
	
	// Day of week (normalized 0-1)
	features[1] = float64(now.Weekday()) / 7.0
	
	// Time since last access (hours, log-transformed)
	if len(pattern.AccessTimes) > 0 {
		lastAccess := pattern.AccessTimes[len(pattern.AccessTimes)-1]
		hoursSinceAccess := now.Sub(lastAccess).Hours()
		features[2] = math.Log(1.0 + hoursSinceAccess) / math.Log(168.0) // Normalize by week
	}
	
	// Weekly pattern strength
	totalWeeklyAccesses := 0
	maxDayAccesses := 0
	for _, count := range pattern.WeeklyPattern {
		totalWeeklyAccesses += count
		if count > maxDayAccesses {
			maxDayAccesses = count
		}
	}
	if totalWeeklyAccesses > 0 {
		features[3] = float64(maxDayAccesses) / float64(totalWeeklyAccesses)
	}
	
	return features
}

func (tfe *TemporalFeatureExtractor) GetFeatureNames() []string {
	return []string{"hour_of_day", "day_of_week", "hours_since_access", "weekly_pattern_strength"}
}

// FrequencyFeatureExtractor extracts frequency-based features
type FrequencyFeatureExtractor struct{}

func NewFrequencyFeatureExtractor() *FrequencyFeatureExtractor {
	return &FrequencyFeatureExtractor{}
}

func (ffe *FrequencyFeatureExtractor) ExtractFeatures(pattern *AdaptiveAccessPattern, metadata map[string]interface{}) []float64 {
	features := make([]float64, 3)
	
	// Access count (log-transformed)
	accessCount := len(pattern.AccessTimes)
	features[0] = math.Log(1.0 + float64(accessCount)) / math.Log(1000.0) // Normalize by 1000
	
	// Average access interval
	if len(pattern.AccessIntervals) > 0 {
		totalInterval := time.Duration(0)
		for _, interval := range pattern.AccessIntervals {
			totalInterval += interval
		}
		avgInterval := totalInterval / time.Duration(len(pattern.AccessIntervals))
		features[1] = math.Log(1.0 + avgInterval.Hours()) / math.Log(168.0) // Normalize by week
	}
	
	// Access regularity (coefficient of variation of intervals)
	if len(pattern.AccessIntervals) > 1 {
		// Calculate mean
		totalInterval := time.Duration(0)
		for _, interval := range pattern.AccessIntervals {
			totalInterval += interval
		}
		mean := float64(totalInterval) / float64(len(pattern.AccessIntervals))
		
		// Calculate standard deviation
		variance := 0.0
		for _, interval := range pattern.AccessIntervals {
			diff := float64(interval) - mean
			variance += diff * diff
		}
		variance /= float64(len(pattern.AccessIntervals))
		stddev := math.Sqrt(variance)
		
		// Coefficient of variation
		if mean > 0 {
			features[2] = stddev / mean
		}
	}
	
	return features
}

func (ffe *FrequencyFeatureExtractor) GetFeatureNames() []string {
	return []string{"access_count", "avg_interval", "interval_regularity"}
}

// RecencyFeatureExtractor extracts recency-based features
type RecencyFeatureExtractor struct{}

func NewRecencyFeatureExtractor() *RecencyFeatureExtractor {
	return &RecencyFeatureExtractor{}
}

func (rfe *RecencyFeatureExtractor) ExtractFeatures(pattern *AdaptiveAccessPattern, metadata map[string]interface{}) []float64 {
	features := make([]float64, 2)
	now := time.Now()
	
	// Recency score (exponential decay)
	if len(pattern.AccessTimes) > 0 {
		lastAccess := pattern.AccessTimes[len(pattern.AccessTimes)-1]
		hoursSinceAccess := now.Sub(lastAccess).Hours()
		features[0] = math.Exp(-hoursSinceAccess / 24.0) // 24-hour half-life
	}
	
	// Recent activity trend (last 5 accesses vs previous 5)
	if len(pattern.AccessTimes) >= 10 {
		recent := pattern.AccessTimes[len(pattern.AccessTimes)-5:]
		previous := pattern.AccessTimes[len(pattern.AccessTimes)-10 : len(pattern.AccessTimes)-5]
		
		recentInterval := recent[len(recent)-1].Sub(recent[0])
		previousInterval := previous[len(previous)-1].Sub(previous[0])
		
		if previousInterval > 0 {
			features[1] = float64(recentInterval) / float64(previousInterval) // Trend ratio
		}
	}
	
	return features
}

func (rfe *RecencyFeatureExtractor) GetFeatureNames() []string {
	return []string{"recency_score", "activity_trend"}
}

// MetadataFeatureExtractor extracts features from block metadata
type MetadataFeatureExtractor struct{}

func NewMetadataFeatureExtractor() *MetadataFeatureExtractor {
	return &MetadataFeatureExtractor{}
}

func (mfe *MetadataFeatureExtractor) ExtractFeatures(pattern *AdaptiveAccessPattern, metadata map[string]interface{}) []float64 {
	features := make([]float64, 3)
	
	// Is randomizer block
	if isRandomizer, ok := metadata["is_randomizer"].(bool); ok && isRandomizer {
		features[0] = 1.0
	}
	
	// Block type encoding
	if blockType, ok := metadata["block_type"].(string); ok {
		switch blockType {
		case "data":
			features[1] = 1.0
		case "descriptor":
			features[1] = 0.8
		case "index":
			features[1] = 0.6
		default:
			features[1] = 0.5
		}
	}
	
	// Preloaded indicator
	if preloaded, ok := metadata["preloaded"].(bool); ok && preloaded {
		features[2] = 1.0
	}
	
	return features
}

func (mfe *MetadataFeatureExtractor) GetFeatureNames() []string {
	return []string{"is_randomizer", "block_type", "is_preloaded"}
}