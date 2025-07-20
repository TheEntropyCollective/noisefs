package integration

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/integration/coordinator/subsystems"
)

// SystemCoordinator orchestrates all NoiseFS components using focused subsystems
type SystemCoordinator struct {
	// Configuration
	config *config.Config

	// Focused subsystems
	storage    *subsystems.StorageSubsystem
	privacy    *subsystems.PrivacySubsystem
	reuse      *subsystems.ReuseSubsystem
	compliance *subsystems.ComplianceSubsystem
	metrics    *subsystems.MetricsSubsystem

	// Core components  
	noisefsClient   *noisefs.Client
	descriptorStore *descriptors.Store
}

// SystemMetrics is re-exported from the metrics subsystem for backward compatibility
type SystemMetrics = subsystems.SystemMetrics

// NewSystemCoordinator creates a new system coordinator with all subsystems
func NewSystemCoordinator(cfg *config.Config) (*SystemCoordinator, error) {
	coordinator := &SystemCoordinator{
		config: cfg,
	}

	// Initialize subsystems in dependency order
	var err error

	// Storage subsystem (foundation)
	coordinator.storage, err = subsystems.NewStorageSubsystem(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage subsystem: %w", err)
	}

	// Privacy subsystem (depends on storage for cache)
	coordinator.privacy, err = subsystems.NewPrivacySubsystem(coordinator.storage.GetBlockCache())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize privacy subsystem: %w", err)
	}

	// Reuse subsystem (depends on storage)
	coordinator.reuse, err = subsystems.NewReuseSubsystem(coordinator.storage.GetStorageManager(), coordinator.storage.GetBlockCache())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize reuse subsystem: %w", err)
	}

	// Compliance subsystem (independent)
	coordinator.compliance, err = subsystems.NewComplianceSubsystem()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize compliance subsystem: %w", err)
	}

	// Metrics subsystem (independent)
	coordinator.metrics = subsystems.NewMetricsSubsystem()

	// Initialize core components
	if err := coordinator.initializeCore(); err != nil {
		return nil, fmt.Errorf("failed to initialize core: %w", err)
	}

	// Wire components together
	if err := coordinator.wireComponents(); err != nil {
		return nil, fmt.Errorf("failed to wire components: %w", err)
	}

	return coordinator, nil
}


// initializeCore sets up core NoiseFS components
func (sc *SystemCoordinator) initializeCore() error {
	// Block management is handled by the blocks package directly

	// Create NoiseFS client using storage subsystem
	var err error
	sc.noisefsClient, err = noisefs.NewClient(sc.storage.GetStorageManager(), sc.storage.GetBlockCache())
	if err != nil {
		return fmt.Errorf("failed to create NoiseFS client: %w", err)
	}

	// Create descriptor store using storage subsystem
	sc.descriptorStore, err = descriptors.NewStore(sc.storage.GetStorageManager())
	if err != nil {
		return fmt.Errorf("failed to create descriptor store: %w", err)
	}

	return nil
}

// wireComponents connects all components together
func (sc *SystemCoordinator) wireComponents() error {
	// Skip peer manager wiring since we don't have it initialized
	// In a real implementation, this would require libp2p host setup

	// Start privacy subsystem
	ctx := context.Background()
	if err := sc.privacy.Start(ctx); err != nil {
		return fmt.Errorf("failed to start privacy subsystem: %w", err)
	}

	return nil
}

// UploadFile performs a complete file upload with all protections
func (sc *SystemCoordinator) UploadFile(reader io.Reader, filename string) (string, error) {
	// Update metrics
	sc.metrics.IncrementUploads()

	// Use reuse-aware client for upload with default block size
	result, err := sc.reuse.GetReuseClient().UploadFile(reader, filename, blocks.DefaultBlockSize)
	if err != nil {
		return "", fmt.Errorf("upload failed: %w", err)
	}

	// Log compliance event
	err = sc.compliance.GetComplianceAudit().LogComplianceEvent(
		"file_upload",
		"system",
		result.DescriptorCID,
		"upload_completed",
		map[string]interface{}{
			"filename":      filename,
			"reuse_proof":   result.ReuseProof,
			"mixing_plan":   result.MixingPlan,
			"privacy_score": sc.metrics.GetSystemMetrics(sc.reuse.GetReuseClient(), sc.privacy.GetCoverTraffic(), sc.noisefsClient).PrivacyScore,
		},
	)
	if err != nil {
		// Log error but don't fail upload
		fmt.Printf("Warning: failed to log compliance event: %v\n", err)
	}

	// Update system metrics
	sc.metrics.UpdateMetricsFromUpload(result, sc.privacy.GetCoverTraffic())

	return result.DescriptorCID, nil
}

// DownloadFile performs a complete file download with privacy
func (sc *SystemCoordinator) DownloadFile(descriptorCID string) (io.Reader, error) {
	// Update metrics
	sc.metrics.IncrementDownloads()

	// Mix download request with cover traffic
	ctx := context.Background()
	mixedResult, err := sc.privacy.GetRequestMixer().SubmitRequest(ctx, descriptorCID, 1)
	if err != nil {
		// Fallback to direct download if mixing fails
		data, err := sc.reuse.GetReuseClient().DownloadFile(descriptorCID)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(data), nil
	}

	// Use mixed result for download
	if mixedResult.Success && mixedResult.Data != nil {
		// Parse descriptor and download file
		data, err := sc.reuse.GetReuseClient().DownloadFile(descriptorCID)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(data), nil
	}

	return nil, fmt.Errorf("download failed")
}

// GetSystemMetrics returns current system metrics
func (sc *SystemCoordinator) GetSystemMetrics() *SystemMetrics {
	return sc.metrics.GetSystemMetrics(sc.reuse.GetReuseClient(), sc.privacy.GetCoverTraffic(), sc.noisefsClient)
}

// Advanced API for accessing subsystems (optional, for power users)

// GetStorageSubsystem returns the storage subsystem for advanced storage operations
func (sc *SystemCoordinator) GetStorageSubsystem() *subsystems.StorageSubsystem {
	return sc.storage
}

// GetPrivacySubsystem returns the privacy subsystem for advanced privacy operations
func (sc *SystemCoordinator) GetPrivacySubsystem() *subsystems.PrivacySubsystem {
	return sc.privacy
}

// GetReuseSubsystem returns the reuse subsystem for advanced reuse operations
func (sc *SystemCoordinator) GetReuseSubsystem() *subsystems.ReuseSubsystem {
	return sc.reuse
}

// GetComplianceSubsystem returns the compliance subsystem for advanced compliance operations
func (sc *SystemCoordinator) GetComplianceSubsystem() *subsystems.ComplianceSubsystem {
	return sc.compliance
}

// GetMetricsSubsystem returns the metrics subsystem for advanced metrics operations
func (sc *SystemCoordinator) GetMetricsSubsystem() *subsystems.MetricsSubsystem {
	return sc.metrics
}

// Shutdown gracefully shuts down all subsystems
func (sc *SystemCoordinator) Shutdown() error {
	// Shutdown all subsystems
	if err := sc.privacy.Shutdown(); err != nil {
		return fmt.Errorf("failed to shutdown privacy subsystem: %w", err)
	}

	if err := sc.storage.Shutdown(); err != nil {
		return fmt.Errorf("failed to shutdown storage subsystem: %w", err)
	}

	if err := sc.reuse.Shutdown(); err != nil {
		return fmt.Errorf("failed to shutdown reuse subsystem: %w", err)
	}

	if err := sc.compliance.Shutdown(); err != nil {
		return fmt.Errorf("failed to shutdown compliance subsystem: %w", err)
	}

	if err := sc.metrics.Shutdown(); err != nil {
		return fmt.Errorf("failed to shutdown metrics subsystem: %w", err)
	}

	return nil
}
