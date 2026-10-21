package deepseek

import (
	"testing"
)

func TestDecodeModelCatalogContextLimits(t *testing.T) {
	catalog := &RawModelCatalog{
		Groups: []RawModelGroup{
			{
				ID:   "deepseek",
				Name: "DeepSeek",
				Models: []RawModelEntry{
					{
						ID:            "explicit-window",
						Name:          "Model With Explicit Window",
						ContextWindow: 65536,
					},
					{
						ID:            "deepseek-chat",
						Name:          "DeepSeek Chat",
						ContextWindow: 0, // Unadvertised by dsh
					},
					{
						ID:            "deepseek-v4-flash-vision-exp",
						Name:          "DeepSeek Vision",
						ContextWindow: 0,
					},
					{
						ID:            "custom-model-32k",
						Name:          "Custom 32k Model",
						ContextWindow: 0,
					},
					{
						ID:            "other-arbitrary-model",
						Name:          "Other Model",
						ContextWindow: 0,
					},
				},
			},
		},
	}

	models := DecodeModelCatalog(catalog)
	if len(models) != 5 {
		t.Fatalf("len(models) = %d, want 5", len(models))
	}

	// 1. Explicit context window is honored
	if models[0].ContextLimit == nil || *models[0].ContextLimit != 65536 {
		t.Errorf("model[0].ContextLimit = %v, want 65536", models[0].ContextLimit)
	}

	// 2. deepseek-chat defaults to 128000
	if models[1].ContextLimit == nil || *models[1].ContextLimit != 128000 {
		t.Errorf("model[1].ContextLimit = %v, want 128000", models[1].ContextLimit)
	}

	// 3. Vision model defaults to 1000000
	if models[2].ContextLimit == nil || *models[2].ContextLimit != 1000000 {
		t.Errorf("model[2].ContextLimit = %v, want 1000000", models[2].ContextLimit)
	}

	// 4. Model with 32k in name defaults to 32000
	if models[3].ContextLimit == nil || *models[3].ContextLimit != 32000 {
		t.Errorf("model[3].ContextLimit = %v, want 32000", models[3].ContextLimit)
	}

	// 5. Arbitrary unadvertised model defaults to 128000
	if models[4].ContextLimit == nil || *models[4].ContextLimit != 128000 {
		t.Errorf("model[4].ContextLimit = %v, want 128000", models[4].ContextLimit)
	}
}

func TestDecodeModelCatalogNil(t *testing.T) {
	if got := DecodeModelCatalog(nil); got != nil {
		t.Errorf("DecodeModelCatalog(nil) = %v, want nil", got)
	}
}
