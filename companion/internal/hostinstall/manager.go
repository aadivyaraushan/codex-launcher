package hostinstall

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall/operationlock"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall/transaction"
)

const maxArtifactBytes int64 = 512 * 1024 * 1024
const maxConfigBytes int64 = 1024 * 1024

var (
	ErrArtifactVerification    = errors.New("local companion artifact could not be verified")
	ErrInstallNeedsRepair      = errors.New("companion install failed and automatic cleanup also failed")
	ErrNotInstalled            = errors.New("companion is not installed")
	ErrNoRollback              = errors.New("no previous companion is available")
	ErrReplacementRolledBack   = errors.New("companion replacement failed and the prior version was restored")
	ErrReplacementNeedsRepair  = errors.New("companion replacement failed and automatic restore also failed")
	ErrRollbackRolledBack      = errors.New("companion rollback failed and the newer version was restored")
	ErrRollbackNeedsRepair     = errors.New("companion rollback failed and automatic restore also failed")
	ErrMaintenanceScheduled    = errors.New("companion maintenance was scheduled after this process exits")
	ErrReconfigureNeedsRepair  = errors.New("companion reconfiguration failed and automatic restore also failed")
	ErrInvalidInstallArguments = errors.New("companion install arguments are invalid")
)

type Provenance struct {
	Artifact     string `json:"artifact"`
	SHA256       string `json:"sha256"`
	Version      string `json:"version"`
	GOOS         string `json:"goos"`
	GOARCH       string `json:"goarch"`
	SourceCommit string `json:"sourceCommit"`
}

type ServiceStatus struct {
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Detail    string `json:"detail,omitempty"`
}

type Backend interface {
	Install(context.Context, string) error
	Start(context.Context) error
	Stop(context.Context) error
	Remove(context.Context) error
	Status(context.Context) (ServiceStatus, error)
}

type ConfigMigration func() (rollback func() error, err error)

type StartHealth interface {
	PrepareStart() (string, error)
	WaitRunning(context.Context, string) error
}

type DeferredMutator interface {
	DeferIfRunningInstalled(operation, artifact, installedPath string) (bool, error)
}

type Options struct {
	Root             string
	SourceExecutable string
	Backend          Backend
	Logger           *slog.Logger
	GOOS             string
	GOARCH           string
	ValidateBinary   func(string) error
	MigrateConfig    ConfigMigration
	StartHealth      StartHealth
	DeferredMutator  DeferredMutator
}

type Manager struct {
	root             string
	sourceExecutable string
	backend          Backend
	logger           *slog.Logger
	goos             string
	goarch           string
	validateBinary   func(string) error
	migrateConfig    ConfigMigration
	startHealth      StartHealth
	deferredMutator  DeferredMutator
}

func New(options Options) *Manager {
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	goos := options.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := options.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	migrate := options.MigrateConfig
	if migrate == nil {
		migrate = func() (func() error, error) { return func() error { return nil }, nil }
	}
	return &Manager{
		root: options.Root, sourceExecutable: options.SourceExecutable, backend: options.Backend,
		logger: logger, goos: goos, goarch: goarch, validateBinary: options.ValidateBinary, migrateConfig: migrate, startHealth: options.StartHealth, deferredMutator: options.DeferredMutator,
	}
}

func (manager *Manager) BinaryPath() string {
	return filepath.Join(manager.root, "bin", executableName(manager.goos))
}

func (manager *Manager) PreviousBinaryPath() string {
	return filepath.Join(manager.root, "bin", executableName(manager.goos)+".previous")
}

func (manager *Manager) ConfigPath() string { return filepath.Join(manager.root, "config.json") }

func (manager *Manager) PreviousConfigPath() string {
	return filepath.Join(manager.root, "config.json.previous")
}

func (manager *Manager) transactionDir() string {
	return filepath.Join(manager.root, "install-transaction")
}

func (manager *Manager) transactionBinaryPath() string {
	return filepath.Join(manager.transactionDir(), "binary")
}

func (manager *Manager) transactionConfigPath() string {
	return filepath.Join(manager.transactionDir(), "config.json")
}

func (manager *Manager) transactionConfigAbsentPath() string {
	return filepath.Join(manager.transactionDir(), "config.absent")
}

func (manager *Manager) previousConfigAbsentPath() string {
	return filepath.Join(manager.root, "config.json.previous.absent")
}

func (manager *Manager) Install(ctx context.Context) error {
	return manager.withMutation(ctx, func() error { return manager.install(ctx) })
}

func (manager *Manager) install(ctx context.Context) error {
	manager.logger.Info("[host-install] install requested", "input_shape", "current_executable", "platform", manager.goos)
	if manager.root == "" || manager.sourceExecutable == "" || manager.backend == nil || manager.validateBinary == nil {
		return ErrInvalidInstallArguments
	}
	if _, err := os.Stat(manager.BinaryPath()); err == nil {
		return fmt.Errorf("%w: installed binary already exists", ErrInvalidInstallArguments)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := validateSourceFile(manager.sourceExecutable); err != nil {
		return err
	}
	if err := manager.validateBinary(manager.sourceExecutable); err != nil {
		return fmt.Errorf("%w: running binary validation: %v", ErrArtifactVerification, err)
	}
	if err := manager.prepareRoot(); err != nil {
		return err
	}
	if err := manager.prepareTransaction(false); err != nil {
		return err
	}
	if err := manager.writeTransaction(transaction.OperationInstall, transaction.PhasePrepared); err != nil {
		return err
	}
	if err := copyAtomic(manager.sourceExecutable, manager.BinaryPath()); err != nil {
		return manager.cleanupFailedInstall(ctx, err)
	}
	if err := manager.writeTransaction(transaction.OperationInstall, transaction.PhaseActivated); err != nil {
		return manager.cleanupFailedInstall(ctx, err)
	}
	if err := manager.backend.Install(ctx, manager.BinaryPath()); err != nil {
		return manager.cleanupFailedInstall(ctx, err)
	}
	if err := manager.startAndVerify(ctx); err != nil {
		return manager.cleanupFailedInstall(ctx, err)
	}
	if err := manager.writeTransaction(transaction.OperationInstall, transaction.PhaseHealthy); err != nil {
		return fmt.Errorf("%w: record healthy install: %v", ErrInstallNeedsRepair, err)
	}
	if err := manager.finishTransaction(); err != nil {
		return fmt.Errorf("%w: clear install transaction: %v", ErrInstallNeedsRepair, err)
	}
	manager.logger.Info("[host-install] install completed", "output_shape", "binary_and_user_service", "platform", manager.goos)
	return nil
}

func (manager *Manager) cleanupFailedInstall(ctx context.Context, cause error) error {
	stopErr := manager.backend.Stop(ctx)
	removeServiceErr := manager.backend.Remove(ctx)
	removeBinaryErr := os.Remove(manager.BinaryPath())
	if errors.Is(removeBinaryErr, os.ErrNotExist) {
		removeBinaryErr = nil
	}
	if stopErr != nil || removeServiceErr != nil || removeBinaryErr != nil {
		manager.logger.Error("[host-install] automatic install cleanup failed", "error_class", "install_cleanup", "service_stop_failed", stopErr != nil, "service_remove_failed", removeServiceErr != nil, "binary_remove_failed", removeBinaryErr != nil)
		return fmt.Errorf("%w: %w", ErrInstallNeedsRepair, errors.Join(cause, stopErr, removeServiceErr, removeBinaryErr))
	}
	if err := manager.finishTransaction(); err != nil {
		return fmt.Errorf("%w: install=%v, clear transaction=%v", ErrInstallNeedsRepair, cause, err)
	}
	manager.logger.Error("[host-install] failed install cleaned up", "error_class", "install", "decision", "partial_service_removed")
	return cause
}

func (manager *Manager) Replace(ctx context.Context, artifactPath string) error {
	return manager.withMutation(ctx, func() error { return manager.replace(ctx, artifactPath) })
}

func (manager *Manager) replace(ctx context.Context, artifactPath string) error {
	manager.logger.Info("[host-install] replacement requested", "input_shape", "local_artifact_with_checksum_and_provenance", "platform", manager.goos)
	if deferred, err := manager.deferMutation("replace", artifactPath); err != nil || deferred {
		return err
	}
	if manager.backend == nil || manager.validateBinary == nil {
		return ErrInvalidInstallArguments
	}
	if _, err := os.Stat(manager.BinaryPath()); err != nil {
		return ErrNotInstalled
	}
	staged := manager.BinaryPath() + ".replacement"
	_ = os.Remove(staged)
	if _, err := stageVerifiedArtifact(artifactPath, staged, manager.goos, manager.goarch); err != nil {
		manager.logger.Error("[host-install] replacement verification failed", "error_class", "artifact_verification")
		return err
	}
	defer os.Remove(staged)
	if err := manager.validateBinary(staged); err != nil {
		return fmt.Errorf("%w: candidate binary validation: %v", ErrArtifactVerification, err)
	}
	if err := manager.prepareTransaction(true); err != nil {
		return err
	}
	if err := manager.writeTransaction(transaction.OperationReplace, transaction.PhasePrepared); err != nil {
		return err
	}
	if err := manager.backend.Stop(ctx); err != nil {
		return manager.restoreReplacement(ctx, nil, err)
	}
	if err := manager.activateReplacement(staged); err != nil {
		return manager.restoreReplacement(ctx, nil, err)
	}
	if err := manager.writeTransaction(transaction.OperationReplace, transaction.PhaseActivated); err != nil {
		return manager.restoreReplacement(ctx, nil, err)
	}
	rollbackConfig, err := manager.migrateConfig()
	if err != nil {
		return manager.restoreReplacement(ctx, rollbackConfig, err)
	}
	if err := manager.backend.Install(ctx, manager.BinaryPath()); err != nil {
		return manager.restoreReplacement(ctx, rollbackConfig, err)
	}
	if err := manager.startAndVerify(ctx); err != nil {
		return manager.restoreReplacement(ctx, rollbackConfig, err)
	}
	if err := manager.writeTransaction(transaction.OperationReplace, transaction.PhaseHealthy); err != nil {
		return fmt.Errorf("%w: record healthy replacement: %v", ErrReplacementNeedsRepair, err)
	}
	if err := manager.publishTransactionAsRollback(); err != nil {
		return fmt.Errorf("%w: publish replacement rollback: %v", ErrReplacementNeedsRepair, err)
	}
	if err := manager.finishTransaction(); err != nil {
		return fmt.Errorf("%w: finish replacement transaction: %v", ErrReplacementNeedsRepair, err)
	}
	manager.logger.Info("[host-install] replacement completed", "output_shape", "new_binary_plus_one_rollback", "platform", manager.goos)
	return nil
}

func (manager *Manager) Rollback(ctx context.Context) error {
	return manager.withMutation(ctx, func() error { return manager.rollback(ctx) })
}

func (manager *Manager) rollback(ctx context.Context) error {
	manager.logger.Info("[host-install] rollback requested", "input_shape", "one_previous_binary", "platform", manager.goos)
	if deferred, err := manager.deferMutation("rollback", ""); err != nil || deferred {
		return err
	}
	if manager.backend == nil {
		return ErrInvalidInstallArguments
	}
	if _, err := os.Stat(manager.PreviousBinaryPath()); err != nil {
		return ErrNoRollback
	}
	if err := manager.prepareTransaction(true); err != nil {
		return err
	}
	if err := manager.writeTransaction(transaction.OperationRollback, transaction.PhasePrepared); err != nil {
		return err
	}
	if err := manager.backend.Stop(ctx); err != nil {
		return manager.restoreRollback(ctx, err)
	}
	if err := manager.activatePersistentRollback(); err != nil {
		return manager.restoreRollback(ctx, err)
	}
	if err := manager.writeTransaction(transaction.OperationRollback, transaction.PhaseActivated); err != nil {
		return manager.restoreRollback(ctx, err)
	}
	if err := manager.backend.Install(ctx, manager.BinaryPath()); err != nil {
		return manager.restoreRollback(ctx, err)
	}
	if err := manager.startAndVerify(ctx); err != nil {
		return manager.restoreRollback(ctx, err)
	}
	if err := manager.writeTransaction(transaction.OperationRollback, transaction.PhaseHealthy); err != nil {
		return fmt.Errorf("%w: record healthy rollback: %v", ErrRollbackNeedsRepair, err)
	}
	if err := manager.consumePersistentRollback(); err != nil {
		return fmt.Errorf("%w: consume rollback: %v", ErrRollbackNeedsRepair, err)
	}
	if err := manager.finishTransaction(); err != nil {
		return fmt.Errorf("%w: finish rollback transaction: %v", ErrRollbackNeedsRepair, err)
	}
	manager.logger.Info("[host-install] rollback completed", "output_shape", "prior_binary_active", "platform", manager.goos)
	return nil
}

func (manager *Manager) Uninstall(ctx context.Context) error {
	return manager.withMutation(ctx, func() error { return manager.uninstall(ctx) })
}

func (manager *Manager) uninstall(ctx context.Context) error {
	manager.logger.Info("[host-install] uninstall requested", "input_shape", "service_and_private_root", "platform", manager.goos)
	if deferred, err := manager.deferMutation("uninstall", ""); err != nil || deferred {
		return err
	}
	if manager.root == "" || manager.backend == nil {
		return ErrInvalidInstallArguments
	}
	if err := manager.writeTransaction(transaction.OperationUninstall, transaction.PhasePrepared); err != nil {
		return err
	}
	stopErr := manager.backend.Stop(ctx)
	if stopErr != nil {
		return fmt.Errorf("stop companion before uninstall: %w", stopErr)
	}
	if err := manager.writeTransaction(transaction.OperationUninstall, transaction.PhaseStopped); err != nil {
		return err
	}
	if err := manager.backend.Remove(ctx); err != nil {
		return err
	}
	if err := os.RemoveAll(manager.root); err != nil {
		return err
	}
	manager.logger.Info("[host-install] uninstall completed", "output_shape", "service_absent_private_root_absent", "platform", manager.goos)
	return nil
}

func (manager *Manager) deferMutation(operation, artifact string) (bool, error) {
	if manager.deferredMutator == nil {
		return false, nil
	}
	deferred, err := manager.deferredMutator.DeferIfRunningInstalled(operation, artifact, manager.BinaryPath())
	if err != nil {
		return false, fmt.Errorf("schedule deferred companion maintenance: %w", err)
	}
	if deferred {
		manager.logger.Info("[host-install] mutation deferred until process exit", "operation", operation, "platform", manager.goos)
		return true, ErrMaintenanceScheduled
	}
	return false, nil
}

func (manager *Manager) Status(ctx context.Context) (ServiceStatus, error) {
	if manager.backend == nil {
		return ServiceStatus{}, ErrInvalidInstallArguments
	}
	return manager.backend.Status(ctx)
}

func (manager *Manager) Reconfigure(ctx context.Context, apply func() error) error {
	return manager.withMutation(ctx, func() error { return manager.reconfigure(ctx, apply) })
}

func (manager *Manager) reconfigure(ctx context.Context, apply func() error) error {
	if manager.backend == nil || apply == nil {
		return ErrInvalidInstallArguments
	}
	status, err := manager.backend.Status(ctx)
	if err != nil {
		return err
	}
	if !status.Running {
		return apply()
	}
	if err := manager.prepareTransaction(false); err != nil {
		return fmt.Errorf("backup config before reconfigure: %w", err)
	}
	if err := manager.snapshotTransactionConfig(); err != nil {
		return fmt.Errorf("backup config before reconfigure: %w", err)
	}
	if err := manager.writeTransaction(transaction.OperationReconfigure, transaction.PhasePrepared); err != nil {
		return err
	}
	if err := manager.backend.Stop(ctx); err != nil {
		return err
	}
	if err := manager.writeTransaction(transaction.OperationReconfigure, transaction.PhaseStopped); err != nil {
		return manager.restoreReconfigure(ctx, err)
	}
	if applyErr := apply(); applyErr != nil {
		return manager.restoreReconfigure(ctx, applyErr)
	}
	if startErr := manager.startAndVerify(ctx); startErr != nil {
		return manager.restoreReconfigure(ctx, startErr)
	}
	if err := manager.writeTransaction(transaction.OperationReconfigure, transaction.PhaseHealthy); err != nil {
		return fmt.Errorf("%w: record healthy reconfigure: %v", ErrReconfigureNeedsRepair, err)
	}
	if err := manager.finishTransaction(); err != nil {
		return fmt.Errorf("%w: clear reconfigure transaction: %v", ErrReconfigureNeedsRepair, err)
	}
	return nil
}

func (manager *Manager) withMutation(ctx context.Context, operation func() error) error {
	if manager.root == "" {
		return ErrInvalidInstallArguments
	}
	release, err := operationlock.Acquire(manager.root + ".install.lock")
	if err != nil {
		return err
	}
	operationErr := manager.recoverInterrupted(ctx)
	if operationErr == nil {
		operationErr = operation()
	}
	releaseErr := release()
	if releaseErr != nil {
		manager.logger.Error("[host-install] operation lock release failed", "error_class", "install_lock_release")
	}
	return errors.Join(operationErr, releaseErr)
}

func (manager *Manager) Recover(ctx context.Context) error {
	if manager.root == "" {
		return ErrInvalidInstallArguments
	}
	release, err := operationlock.Acquire(manager.root + ".install.lock")
	if err != nil {
		return err
	}
	recoveryErr := manager.recoverInterrupted(ctx)
	return errors.Join(recoveryErr, release())
}

func (manager *Manager) journal() *transaction.Store {
	return transaction.New(filepath.Join(manager.root, "install-transaction.json"))
}

func (manager *Manager) recoverInterrupted(ctx context.Context) error {
	record, err := manager.journal().Read()
	if errors.Is(err, transaction.ErrMissing) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: unreadable install transaction", ErrInstallNeedsRepair)
	}
	manager.logger.Info("[host-install] interrupted operation recovery started", "operation", record.Operation, "phase", record.Phase)
	if manager.backend == nil {
		return ErrInvalidInstallArguments
	}
	var recoveryErr error
	switch record.Operation {
	case transaction.OperationInstall:
		if record.Phase == transaction.PhaseHealthy {
			recoveryErr = manager.finishTransaction()
		} else {
			stopErr := manager.backend.Stop(ctx)
			removeErr := manager.backend.Remove(ctx)
			binaryErr := removeIfPresent(manager.BinaryPath())
			if stopErr == nil && removeErr == nil && binaryErr == nil {
				recoveryErr = manager.finishTransaction()
			} else {
				recoveryErr = errors.Join(stopErr, removeErr, binaryErr)
			}
		}
	case transaction.OperationReplace:
		if record.Phase == transaction.PhaseHealthy {
			if recoveryErr = manager.publishTransactionAsRollback(); recoveryErr == nil {
				recoveryErr = manager.finishTransaction()
			}
		} else if recoveryErr = manager.restoreTransactionSnapshot(ctx); recoveryErr == nil {
			recoveryErr = manager.finishTransaction()
		}
	case transaction.OperationRollback:
		if record.Phase == transaction.PhaseHealthy {
			if recoveryErr = manager.consumePersistentRollback(); recoveryErr == nil {
				recoveryErr = manager.finishTransaction()
			}
		} else if recoveryErr = manager.restoreTransactionSnapshot(ctx); recoveryErr == nil {
			recoveryErr = manager.finishTransaction()
		}
	case transaction.OperationReconfigure:
		if record.Phase == transaction.PhaseHealthy {
			recoveryErr = manager.finishTransaction()
		} else {
			stopErr := manager.backend.Stop(ctx)
			restoreErr := manager.restoreTransactionConfig()
			startErr := error(nil)
			if stopErr == nil && restoreErr == nil {
				startErr = manager.startAndVerify(ctx)
			}
			if stopErr == nil && restoreErr == nil && startErr == nil {
				recoveryErr = manager.finishTransaction()
			} else {
				recoveryErr = errors.Join(stopErr, restoreErr, startErr)
			}
		}
	case transaction.OperationUninstall:
		stopErr := manager.backend.Stop(ctx)
		removeErr := error(nil)
		rootErr := error(nil)
		if stopErr == nil {
			removeErr = manager.backend.Remove(ctx)
		}
		if stopErr == nil && removeErr == nil {
			rootErr = os.RemoveAll(manager.root)
		}
		recoveryErr = errors.Join(stopErr, removeErr, rootErr)
	default:
		recoveryErr = ErrInstallNeedsRepair
	}
	if recoveryErr != nil {
		return fmt.Errorf("%w: interrupted %s recovery: %v", ErrInstallNeedsRepair, record.Operation, recoveryErr)
	}
	manager.logger.Info("[host-install] interrupted operation recovery completed", "operation", record.Operation)
	return nil
}

func (manager *Manager) activateReplacement(staged string) error {
	if manager.goos == "windows" {
		if err := removeIfPresent(manager.BinaryPath()); err != nil {
			return err
		}
	}
	return os.Rename(staged, manager.BinaryPath())
}

func (manager *Manager) prepareTransaction(includeBinary bool) error {
	if err := os.RemoveAll(manager.transactionDir()); err != nil {
		return err
	}
	if err := os.MkdirAll(manager.transactionDir(), 0o700); err != nil {
		return err
	}
	if includeBinary {
		if err := copyAtomic(manager.BinaryPath(), manager.transactionBinaryPath()); err != nil {
			return fmt.Errorf("snapshot active companion: %w", err)
		}
	}
	return manager.snapshotTransactionConfig()
}

func (manager *Manager) snapshotTransactionConfig() error {
	_ = removeIfPresent(manager.transactionConfigPath())
	_ = removeIfPresent(manager.transactionConfigAbsentPath())
	info, err := os.Lstat(manager.ConfigPath())
	if errors.Is(err, os.ErrNotExist) {
		return os.WriteFile(manager.transactionConfigAbsentPath(), []byte("absent\n"), 0o600)
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxConfigBytes {
		return errors.New("active config is not a bounded regular file")
	}
	return copyPrivateAtomic(manager.ConfigPath(), manager.transactionConfigPath(), maxConfigBytes)
}

func (manager *Manager) restoreTransactionConfig() error {
	if _, err := os.Lstat(manager.transactionConfigAbsentPath()); err == nil {
		return removeIfPresent(manager.ConfigPath())
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return copyPrivateAtomic(manager.transactionConfigPath(), manager.ConfigPath(), maxConfigBytes)
}

func (manager *Manager) restoreTransactionSnapshot(ctx context.Context) error {
	stopErr := manager.backend.Stop(ctx)
	binaryErr := copyAtomic(manager.transactionBinaryPath(), manager.BinaryPath())
	configErr := manager.restoreTransactionConfig()
	installErr := error(nil)
	startErr := error(nil)
	if stopErr == nil && binaryErr == nil && configErr == nil {
		installErr = manager.backend.Install(ctx, manager.BinaryPath())
		if installErr == nil {
			startErr = manager.startAndVerify(ctx)
		}
	}
	return errors.Join(stopErr, binaryErr, configErr, installErr, startErr)
}

func (manager *Manager) restoreReplacement(ctx context.Context, rollbackConfig func() error, cause error) error {
	configRollbackErr := error(nil)
	if rollbackConfig != nil {
		configRollbackErr = rollbackConfig()
	}
	restoreErr := manager.restoreTransactionSnapshot(ctx)
	if configRollbackErr != nil || restoreErr != nil {
		manager.logger.Error("[host-install] automatic replacement restore failed", "error_class", "replacement_restore", "config_rollback_failed", configRollbackErr != nil, "snapshot_restore_failed", restoreErr != nil)
		return fmt.Errorf("%w: replacement=%v: %v", ErrReplacementNeedsRepair, cause, errors.Join(configRollbackErr, restoreErr))
	}
	if err := manager.finishTransaction(); err != nil {
		return fmt.Errorf("%w: replacement=%v, finish transaction=%v", ErrReplacementNeedsRepair, cause, err)
	}
	manager.logger.Error("[host-install] replacement rolled back", "error_class", "replacement", "decision", "prior_binary_restored")
	return fmt.Errorf("%w: %v", ErrReplacementRolledBack, cause)
}

func (manager *Manager) restoreRollback(ctx context.Context, cause error) error {
	if err := manager.restoreTransactionSnapshot(ctx); err != nil {
		manager.logger.Error("[host-install] automatic rollback restore failed", "error_class", "rollback_restore", "snapshot_restore_failed", true)
		return fmt.Errorf("%w: rollback=%v: %v", ErrRollbackNeedsRepair, cause, err)
	}
	if err := manager.finishTransaction(); err != nil {
		return fmt.Errorf("%w: rollback=%v, finish transaction=%v", ErrRollbackNeedsRepair, cause, err)
	}
	manager.logger.Error("[host-install] rollback request was reversed", "error_class", "rollback", "decision", "newer_binary_restored")
	return fmt.Errorf("%w: %v", ErrRollbackRolledBack, cause)
}

func (manager *Manager) restoreReconfigure(ctx context.Context, cause error) error {
	stopErr := manager.backend.Stop(ctx)
	restoreErr := error(nil)
	restartErr := error(nil)
	if stopErr == nil {
		restoreErr = manager.restoreTransactionConfig()
	}
	if stopErr == nil && restoreErr == nil {
		restartErr = manager.startAndVerify(ctx)
	}
	if stopErr != nil || restoreErr != nil || restartErr != nil {
		return fmt.Errorf("%w: %w", ErrReconfigureNeedsRepair, errors.Join(cause, stopErr, restoreErr, restartErr))
	}
	if err := manager.finishTransaction(); err != nil {
		return fmt.Errorf("%w: %w", ErrReconfigureNeedsRepair, errors.Join(cause, err))
	}
	return fmt.Errorf("reconfiguration failed; prior config restored: %w", cause)
}

func (manager *Manager) publishTransactionAsRollback() error {
	if err := copyAtomic(manager.transactionBinaryPath(), manager.PreviousBinaryPath()); err != nil {
		return err
	}
	if _, err := os.Lstat(manager.transactionConfigAbsentPath()); err == nil {
		if err := removeIfPresent(manager.PreviousConfigPath()); err != nil {
			return err
		}
		return os.WriteFile(manager.previousConfigAbsentPath(), []byte("absent\n"), 0o600)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := copyPrivateAtomic(manager.transactionConfigPath(), manager.PreviousConfigPath(), maxConfigBytes); err != nil {
		return err
	}
	return removeIfPresent(manager.previousConfigAbsentPath())
}

func (manager *Manager) activatePersistentRollback() error {
	if err := copyAtomic(manager.PreviousBinaryPath(), manager.BinaryPath()); err != nil {
		return err
	}
	if _, err := os.Lstat(manager.previousConfigAbsentPath()); err == nil {
		return removeIfPresent(manager.ConfigPath())
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Lstat(manager.PreviousConfigPath()); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return copyPrivateAtomic(manager.PreviousConfigPath(), manager.ConfigPath(), maxConfigBytes)
}

func (manager *Manager) consumePersistentRollback() error {
	return errors.Join(
		removeIfPresent(manager.PreviousBinaryPath()),
		removeIfPresent(manager.PreviousConfigPath()),
		removeIfPresent(manager.previousConfigAbsentPath()),
	)
}

func (manager *Manager) writeTransaction(operation, phase string) error {
	return manager.journal().Write(transaction.Record{Version: 1, Operation: operation, Phase: phase})
}

func (manager *Manager) finishTransaction() error {
	if err := manager.journal().Clear(); err != nil {
		return err
	}
	return os.RemoveAll(manager.transactionDir())
}

func removeIfPresent(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (manager *Manager) startAndVerify(ctx context.Context) error {
	if manager.startHealth == nil {
		return manager.backend.Start(ctx)
	}
	attemptID, err := manager.startHealth.PrepareStart()
	if err != nil {
		return fmt.Errorf("prepare companion start health check: %w", err)
	}
	if err := manager.backend.Start(ctx); err != nil {
		return err
	}
	if err := manager.startHealth.WaitRunning(ctx, attemptID); err != nil {
		return fmt.Errorf("verify companion start health: %w", err)
	}
	return nil
}

func (manager *Manager) prepareRoot() error {
	if err := os.MkdirAll(filepath.Join(manager.root, "bin"), 0o700); err != nil {
		return err
	}
	if err := os.Chmod(manager.root, 0o700); err != nil {
		return err
	}
	return os.Chmod(filepath.Join(manager.root, "bin"), 0o700)
}

func stageVerifiedArtifact(path, destination, goos, goarch string) (Provenance, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxArtifactBytes {
		return Provenance{}, ErrArtifactVerification
	}
	checksumBytes, err := readSmallRegular(path+".sha256", 4096)
	if err != nil {
		return Provenance{}, ErrArtifactVerification
	}
	fields := strings.Fields(string(checksumBytes))
	if len(fields) != 2 || fields[1] != filepath.Base(path) || len(fields[0]) != sha256.Size*2 {
		return Provenance{}, ErrArtifactVerification
	}
	expected, err := hex.DecodeString(fields[0])
	if err != nil {
		return Provenance{}, ErrArtifactVerification
	}
	provenanceBytes, err := readSmallRegular(path+".provenance.json", 64*1024)
	if err != nil {
		return Provenance{}, ErrArtifactVerification
	}
	decoder := json.NewDecoder(strings.NewReader(string(provenanceBytes)))
	decoder.DisallowUnknownFields()
	var provenance Provenance
	if err := decoder.Decode(&provenance); err != nil {
		return Provenance{}, ErrArtifactVerification
	}
	if provenance.Artifact != filepath.Base(path) || !strings.EqualFold(provenance.SHA256, fields[0]) || provenance.Version == "" || provenance.GOOS != goos || provenance.GOARCH != goarch || !validCommit(provenance.SourceCommit) {
		return Provenance{}, ErrArtifactVerification
	}
	input, err := os.Open(path)
	if err != nil {
		return Provenance{}, ErrArtifactVerification
	}
	openedInfo, statErr := input.Stat()
	if statErr != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) || openedInfo.Size() != info.Size() {
		_ = input.Close()
		return Provenance{}, ErrArtifactVerification
	}
	temporary := destination + ".tmp"
	_ = os.Remove(temporary)
	output, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		_ = input.Close()
		return Provenance{}, err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(output, hash), io.LimitReader(input, maxArtifactBytes+1))
	inputCloseErr := input.Close()
	syncErr := output.Sync()
	outputCloseErr := output.Close()
	if copyErr != nil || inputCloseErr != nil || syncErr != nil || outputCloseErr != nil || written != info.Size() || written > maxArtifactBytes || !equalBytes(hash.Sum(nil), expected) {
		_ = os.Remove(temporary)
		return Provenance{}, ErrArtifactVerification
	}
	if err := os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		return Provenance{}, err
	}
	return provenance, nil
}

func readSmallRegular(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, ErrArtifactVerification
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := bufio.NewReader(io.LimitReader(file, limit+1))
	data, err := io.ReadAll(reader)
	if err != nil || int64(len(data)) > limit {
		return nil, ErrArtifactVerification
	}
	return data, nil
}

func copyAtomic(source, destination string) error {
	if err := validateSourceFile(source); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	temporary := destination + ".tmp"
	_ = os.Remove(temporary)
	output, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(output, io.LimitReader(input, maxArtifactBytes+1))
	syncErr := output.Sync()
	closeErr := output.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil || written > maxArtifactBytes {
		_ = os.Remove(temporary)
		if written > maxArtifactBytes {
			return ErrArtifactVerification
		}
		return errors.Join(copyErr, syncErr, closeErr)
	}
	if err := os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func copyPrivateAtomic(source, destination string, limit int64) error {
	info, err := os.Lstat(source)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return errors.New("source is not a bounded regular file")
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	openedInfo, statErr := input.Stat()
	if statErr != nil || !os.SameFile(info, openedInfo) || openedInfo.Size() != info.Size() {
		_ = input.Close()
		return errors.New("source changed while it was opened")
	}
	temporary := destination + ".tmp"
	_ = os.Remove(temporary)
	output, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		_ = input.Close()
		return err
	}
	written, copyErr := io.Copy(output, io.LimitReader(input, limit+1))
	inputCloseErr := input.Close()
	syncErr := output.Sync()
	outputCloseErr := output.Close()
	if copyErr != nil || inputCloseErr != nil || syncErr != nil || outputCloseErr != nil || written != info.Size() || written > limit {
		_ = os.Remove(temporary)
		return errors.Join(copyErr, inputCloseErr, syncErr, outputCloseErr)
	}
	if err := os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func validateSourceFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxArtifactBytes {
		return ErrArtifactVerification
	}
	return nil
}

func executableName(goos string) string {
	if goos == "windows" {
		return "codex-launcher.exe"
	}
	return "codex-launcher"
}

func validCommit(value string) bool {
	if len(value) < 7 || len(value) > 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && len(value)%2 == 0
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var difference byte
	for index := range left {
		difference |= left[index] ^ right[index]
	}
	return difference == 0
}
