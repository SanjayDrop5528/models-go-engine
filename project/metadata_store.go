// Package project manages project configuration, engine lifecycles, and metadata persistence.
//
// File: metadata_store.go
// Usage:
//   Provides serialization, sanitization, and deserialization helpers between in-memory model definitions
//   and persistent database storage maps for project metadata tables.
package project

import (
	"encoding/json"
	"github.com/SanjayDrop5528/models-go-engine/model"
)

// modelConfigToMap serializes a ModelConfig into a generic map suitable for adapter storage.
//
// Purpose:
//   Converts ModelConfig struct into a map for saving into metadata tables.
//
// Where it is used:
//   - Used internally by metadata persistence routines in Engine.
//
// When can it be used:
//   - Call when storing model configuration in database storage.
func modelConfigToMap(cfg *model.ModelConfig) (map[string]any, error) {
	b, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	err = json.Unmarshal(b, &m)
	return m, err
}

// sanitizeMapForJSON unmarshals nested JSON byte slices or strings back into structured data.
//
// Purpose:
//   Decodes database-stored JSON blobs (like references, enums) so they can deserialize into struct fields.
//
// Where it is used:
//   - Used internally by mapToModelConfig and mapToDataModel.
//
// When can it be used:
//   - Call when normalizing database records retrieved from metadata tables.
func sanitizeMapForJSON(m map[string]any) map[string]any {
	cp := make(map[string]any)
	for k, v := range m {
		if v == nil {
			cp[k] = nil
			continue
		}
		switch val := v.(type) {
		case []byte:
			if k == "reference" || k == "enum" || k == "items" {
				var unparsed any
				if json.Unmarshal(val, &unparsed) == nil {
					cp[k] = unparsed
					continue
				}
			}
			cp[k] = string(val)
		case string:
			if k == "reference" || k == "enum" || k == "items" {
				var unparsed any
				if json.Unmarshal([]byte(val), &unparsed) == nil {
					cp[k] = unparsed
					continue
				}
			}
			cp[k] = val
		default:
			cp[k] = v
		}
	}
	return cp
}

// mapToModelConfig unmarshals a database record map into a typed ModelConfig struct.
//
// Purpose:
//   Restores a ModelConfig from database row map data.
//
// Where it is used:
//   - Used internally by RestoreFromDB and ImportLiveMetadata.
//
// When can it be used:
//   - Call when converting retrieved database rows into model configurations.
func mapToModelConfig(m map[string]any) (*model.ModelConfig, error) {
	cp := sanitizeMapForJSON(m)
	b, err := json.Marshal(cp)
	if err != nil {
		return nil, err
	}
	var cfg model.ModelConfig
	err = json.Unmarshal(b, &cfg)
	return &cfg, err
}

// dataModelToMap serializes a DataModel field definition into a map for adapter storage.
//
// Purpose:
//   Converts DataModel struct into a database persistence map.
//
// Where it is used:
//   - Used internally by AddDataModel and LoadModels.
//
// When can it be used:
//   - Call when storing attribute field definitions.
func dataModelToMap(dm *model.DataModel) (map[string]any, error) {
	b, err := json.Marshal(dm)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	err = json.Unmarshal(b, &m)
	return m, err
}

// mapToDataModel unmarshals a database row map into a typed DataModel struct.
//
// Purpose:
//   Restores a DataModel field definition from database records.
//
// Where it is used:
//   - Used internally by RestoreFromDB and ImportLiveMetadata.
//
// When can it be used:
//   - Call when loading field definitions from storage.
func mapToDataModel(m map[string]any) (*model.DataModel, error) {
	cp := sanitizeMapForJSON(m)
	b, err := json.Marshal(cp)
	if err != nil {
		return nil, err
	}
	var dm model.DataModel
	err = json.Unmarshal(b, &dm)
	return &dm, err
}
