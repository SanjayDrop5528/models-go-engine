package project

import (
	"encoding/json"
	"github.com/SanjayDrop5528/models-go-engine/model"
)

func modelConfigToMap(cfg *model.ModelConfig) (map[string]any, error) {
	b, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	err = json.Unmarshal(b, &m)
	return m, err
}

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

func dataModelToMap(dm *model.DataModel) (map[string]any, error) {
	b, err := json.Marshal(dm)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	err = json.Unmarshal(b, &m)
	return m, err
}

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
