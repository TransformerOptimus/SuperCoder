package injection

import (
	coreconfig "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/config/core"
)

func ProvideDefaultConfig() (defaults *coreconfig.DefaultConfig, err error) {
	defaults = &coreconfig.DefaultConfig{
		Config: map[string]any{
			"env": "development",
			"db": map[string]any{
				"host":     "postgres",
				"port":     5432,
				"user":     "superagi",
				"password": "superagi",
				"name":     "supercoder",
			},
			"redis": map[string]any{
				"host": "redis",
				"port": 6379,
				"db":   0,
				"cache": map[string]any{
					"db":  1,
					"ttl": 300,
				},
				"worker": map[string]any{
					"db": 2,
				},
			},
			"falkordb": map[string]any{
				"host":     "falkordb",
				"port":     6379,
				"password": "",
			},
			"qdrant": map[string]any{
				"host": "qdrant",
				"port": 6334,
				"shard": map[string]any{
					"count":  200,
					"prefix": "shard",
				},
			},
			"reviewer": map[string]any{
				"provider": "anthropic",
				"codebase": map[string]any{
					"indexing": false,
				},
				"max": map[string]any{
					"iterations": 4,
				},
				"pricing": map[string]any{
					"input": map[string]any{
						"per": map[string]any{
							"million": 3.0,
						},
					},
					"output": map[string]any{
						"per": map[string]any{
							"million": 15.0,
						},
					},
				},
			},
			"openai": map[string]any{
				"api": map[string]any{
					"key": "",
				},
				"model": "gpt-5-codex",
				"max": map[string]any{
					"tokens": 16384,
				},
				"temperature": 0,
				"embedding": map[string]any{
					"model":      "text-embedding-3-large",
					"dimensions": 3072,
				},
			},
			"anthropic": map[string]any{
				"api": map[string]any{
					"key": "",
				},
				"model": "claude-sonnet-4-6",
				"max": map[string]any{
					"tokens":     16384,
					"iterations": 3,
				},
				"temperature": 0.2,
			},
			"agent": map[string]any{
				"system": map[string]any{
					"prompt": map[string]any{
						"path": "services/context-engine/prompts/system_review.txt",
					},
				},
				"tools": map[string]any{
					"path": "services/context-engine/prompts/tools.json",
				},
			},
			"indexer": map[string]any{
				"enabled": false,
				"streaming": map[string]any{
					"enabled": false,
				},
				"local": map[string]any{
					"allowed": map[string]any{
						"root": "/workspace",
					},
				},
				"chunk": map[string]any{
					"max": map[string]any{
						"tokens": 1024,
					},
					"smart": true,
					"min": map[string]any{
						"merge": map[string]any{
							"tokens": 64,
						},
					},
				},
				"embedding": map[string]any{
					// Caps in-flight OpenAI embed calls process-wide.
					"concurrency": 5,
				},
				"tokens": map[string]any{
					"per": map[string]any{
						"char": 0.33,
					},
				},
				"timeout": 300,
				"skip": map[string]any{
					"dirs": []string{".venv", ".env", ".git", "node_modules", "vendor", "__pycache__", "target", "build", "dist"},
				},
			},
			// Merkle trees persist to a local directory (shared Docker volume
			// across server+worker). Leaving the S3 bucket empty selects the
			// local-disk CAS backend (flock + sha256 versioning).
			"merkle": map[string]any{
				"dir": "/data/merkle",
			},
			"service": map[string]any{
				"name": "supercoder",
				"port": 8106,
			},
		},
	}
	return
}
