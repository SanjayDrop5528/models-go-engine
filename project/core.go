package project

import (
	"context"
	"errors"
	"github.com/SanjayDrop5528/models-go-engine/adapter"
	"github.com/SanjayDrop5528/models-go-engine/model"
	"os"
	"strings"
	"time"
)

// QueryContextConfig holds query execution parameters and context behavior.
type QueryContextConfig struct {
	DefaultTimeout     time.Duration `json:"default_timeout"`
	MaxLimit           int           `json:"max_limit"`
	SlowQueryThreshold time.Duration `json:"slow_query_threshold"`
	IsolationLevel     string        `json:"isolation_level,omitempty"`
	ReadPreference     string        `json:"read_preference,omitempty"`
}

// AdapterConfig holds database adapter configuration at the Project level.
// Connection string is resolved from environment variables or supplied directly.
type AdapterConfig struct {
	AdapterType         string             `json:"adapter_type"`          // "postgres", "mongodb", "mysql", "memory"
	ConnectionStringEnv string             `json:"connection_string_env"` // ENV variable name e.g. "POSTGRES_DSN"
	ConnectionString    string             `json:"connection_string"`     // Resolved connection string
	Database            string             `json:"database"`              // Default database/schema name
	QueryContext        QueryContextConfig `json:"query_context"`         // Query execution configuration
	Options             map[string]any     `json:"options,omitempty"`     // Adapter-specific options (pool sizes, ssl, etc.)
}

// Project represents a project entity that owns adapter configuration, models, and its engine.
type Project struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Description   string        `json:"description,omitempty"`
	AdapterConfig AdapterConfig `json:"adapter_config"`
	Engine        *Engine       `json:"-"`
	CreatedAt     time.Time     `json:"created_at"`
	CreatedBy     string        `json:"created_by,omitempty"`
	UpdatedAt     time.Time     `json:"updated_at"`
	UpdatedBy     string        `json:"updated_by,omitempty"`
}

// ProjectConfig provides options when creating a new Project.
type ProjectConfig struct {
	ID            string
	Name          string
	Description   string
	AdapterConfig AdapterConfig
	CreatedBy     string
}

// New creates an Engine directly from a database adapter with zero configuration needed.
func New(adp adapter.Adapter) *Engine {
	if adp == nil {
		panic("adapter cannot be nil")
	}
	proj := &Project{
		ID:        "default-project",
		Name:      "Default Project",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	engine := NewEngine(proj, adp)
	proj.Engine = engine
	_ = engine.RestoreFromDB(context.Background())
	return engine
}

// NewWithModels creates an Engine loaded with initial ModelConfigs and DataModels.
func NewWithModels(adp adapter.Adapter, configs []*model.ModelConfig, dataModels []*model.DataModel) (*Engine, error) {
	engine := New(adp)
	if err := engine.LoadModels(context.Background(), configs, dataModels); err != nil {
		return nil, err
	}
	return engine, nil
}

// NewProject creates a new Project, resolves the adapter connection from ENV if configured,
// and initializes the project's dedicated Engine.
func NewProject(cfg ProjectConfig, adp adapter.Adapter) (*Project, error) {
	if adp == nil {
		return nil, errors.New("adapter cannot be nil")
	}

	if strings.TrimSpace(cfg.Name) == "" {
		cfg.Name = "Default Project"
	}
	if cfg.ID == "" {
		cfg.ID = strings.ToLower(strings.ReplaceAll(cfg.Name, " ", "-"))
	}

	adapterCfg := cfg.AdapterConfig
	// Resolve connection string from environment if specified
	if adapterCfg.ConnectionStringEnv != "" && adapterCfg.ConnectionString == "" {
		if envVal := os.Getenv(adapterCfg.ConnectionStringEnv); envVal != "" {
			adapterCfg.ConnectionString = envVal
		}
	}
	if adapterCfg.QueryContext.DefaultTimeout == 0 {
		adapterCfg.QueryContext.DefaultTimeout = 30 * time.Second
	}
	if adapterCfg.QueryContext.MaxLimit == 0 {
		adapterCfg.QueryContext.MaxLimit = 1000
	}

	now := time.Now().UTC()
	proj := &Project{
		ID:            cfg.ID,
		Name:          cfg.Name,
		Description:   cfg.Description,
		AdapterConfig: adapterCfg,
		CreatedAt:     now,
		CreatedBy:     cfg.CreatedBy,
		UpdatedAt:     now,
		UpdatedBy:     cfg.CreatedBy,
	}

	// Initialize Engine for the project
	proj.Engine = NewEngine(proj, adp)
	_ = proj.Engine.RestoreFromDB(context.Background())

	return proj, nil
}
