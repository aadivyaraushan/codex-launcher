package projects

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
)

var (
	ErrInvalidProject     = errors.New("project configuration is invalid")
	ErrProjectNotFound    = errors.New("project was not found")
	ErrProjectUnavailable = errors.New("project folder is unavailable")
	ErrUnsafeProjectPath  = errors.New("project path is unsafe")
)

const MaxChoices = 128

type Config struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Path        string `json:"path"`
}

type Choice struct {
	ID          string
	DisplayName string
}

type project struct {
	choice         Choice
	configuredPath string
	canonicalPath  string
	identity       os.FileInfo
}

type Service struct {
	projects map[string]project
	logger   *slog.Logger
}

func New(configs []Config) (*Service, error) {
	return NewWithLogger(configs, slog.Default())
}

func NewWithLogger(configs []Config, logger *slog.Logger) (*Service, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if len(configs) > MaxChoices {
		return nil, ErrInvalidProject
	}
	service := &Service{projects: make(map[string]project, len(configs)), logger: logger}
	for _, config := range configs {
		if !validConfig(config) {
			return nil, ErrInvalidProject
		}
		if _, duplicate := service.projects[config.ID]; duplicate {
			return nil, ErrInvalidProject
		}
		canonical, identity, err := inspectProjectPath(config.Path)
		if err != nil {
			if errors.Is(err, ErrUnsafeProjectPath) {
				return nil, err
			}
			return nil, fmt.Errorf("inspect configured project: %w", ErrProjectUnavailable)
		}
		service.projects[config.ID] = project{
			choice:         Choice{ID: config.ID, DisplayName: strings.TrimSpace(config.DisplayName)},
			configuredPath: config.Path,
			canonicalPath:  canonical,
			identity:       identity,
		}
	}
	logger.Info("[projects] configuration loaded", "project_count", len(service.projects))
	return service, nil
}

func (service *Service) List() []Choice {
	if service == nil {
		return nil
	}
	choices := make([]Choice, 0, len(service.projects))
	for _, project := range service.projects {
		choices = append(choices, project.choice)
	}
	sort.Slice(choices, func(left, right int) bool { return choices[left].ID < choices[right].ID })
	return choices
}

func (service *Service) Resolve(projectID string) (string, error) {
	if service == nil {
		return "", ErrProjectNotFound
	}
	configured, ok := service.projects[projectID]
	if !ok {
		service.logger.Warn("[projects] resolve rejected", "input_shape", "opaque_project_id", "branch_reason", "unknown_id")
		return "", ErrProjectNotFound
	}
	canonical, identity, err := inspectProjectPath(configured.configuredPath)
	if err != nil || canonical != configured.canonicalPath || !os.SameFile(configured.identity, identity) {
		service.logger.Warn("[projects] resolve rejected", "project_id", projectID, "branch_reason", "folder_changed_or_unavailable")
		return "", ErrProjectUnavailable
	}
	service.logger.Debug("[projects] resolved", "project_id", projectID, "output_shape", "canonical_directory")
	return configured.canonicalPath, nil
}
