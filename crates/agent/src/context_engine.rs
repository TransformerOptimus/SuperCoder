use std::sync::LazyLock;

use async_trait::async_trait;
use percent_encoding::{utf8_percent_encode, NON_ALPHANUMERIC};
use serde::{Deserialize, Deserializer, Serialize};

/// Deserialize `null` JSON values as `Default::default()` (e.g., empty Vec).
/// Handles the case where the server returns `"results": null` instead of `[]`.
fn deserialize_null_as_default<'de, D, T>(deserializer: D) -> Result<T, D::Error>
where
    D: Deserializer<'de>,
    T: Default + Deserialize<'de>,
{
    Ok(Option::deserialize(deserializer)?.unwrap_or_default())
}

// ---------------------------------------------------------------------------
// Shared HTTP client (module-local, shorter timeouts than LLM client)
// ---------------------------------------------------------------------------

static HTTP_CLIENT: LazyLock<reqwest::Client> = LazyLock::new(|| {
    reqwest::Client::builder()
        .connect_timeout(std::time::Duration::from_secs(10))
        .timeout(std::time::Duration::from_secs(30))
        .build()
        .expect("Failed to create context engine HTTP client")
});

// ---------------------------------------------------------------------------
// Error
// ---------------------------------------------------------------------------

#[derive(Debug, thiserror::Error)]
pub enum ContextEngineError {
    #[error("Context engine unavailable: {0}")]
    Unavailable(String),

    #[error("HTTP error {status}: {body}")]
    HttpError { status: u16, body: String },

    #[error("Deserialization error: {0}")]
    DeserializeError(String),
}

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

/// Configuration for connecting to the context engine.
/// Carried on AgentConfig, used to construct ContextEngineClient.
#[derive(Debug, Clone)]
pub struct ContextEngineConfig {
    /// Base URL (e.g., "http://localhost:8080")
    pub base_url: String,
    /// User ID for data isolation
    pub user_id: String,
    /// Workspace ID for data isolation
    pub workspace_id: u64,
    /// Machine ID (UUID from SQLite)
    pub machine_id: String,
    /// Canonical repo path (main checkout, NOT worktree)
    pub repo_path: String,
    /// Auth token (X-Auth-Token header). Empty = no auth.
    pub auth_token: String,
}

// ---------------------------------------------------------------------------
// Request / Response types
// ---------------------------------------------------------------------------

// --- Search ---

#[derive(Debug, Clone, Serialize)]
pub struct SearchRequest {
    pub query: String,
    pub repo_path: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub strategy: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub limit: Option<u32>,
    pub user_id: String,
    pub workspace_id: u64,
    pub machine_id: String,
}

#[derive(Debug, Clone, Deserialize)]
pub struct SearchResponse {
    pub query: String,
    pub total: u32,
    #[serde(default, deserialize_with = "deserialize_null_as_default")]
    pub results: Vec<SearchResultItem>,
    #[serde(default)]
    pub indexing: bool,
    #[serde(default)]
    pub message: Option<String>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct SearchResultItem {
    pub chunk_id: String,
    pub content: String,
    pub file_path: String,
    pub language: String,
    pub score: f32,
    pub source: String,
}

// --- Graph ---

#[derive(Debug, Clone, Serialize)]
pub struct GraphQueryRequest {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub query: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub function_name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub file_path: Option<String>,
    pub repo_path: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub query_type: Option<String>,
    pub user_id: String,
    pub workspace_id: u64,
    pub machine_id: String,
}

#[derive(Debug, Clone, Deserialize)]
pub struct GraphQueryResponse {
    pub total: u32,
    #[serde(default, deserialize_with = "deserialize_null_as_default")]
    pub results: Vec<GraphResultItem>,
    #[serde(default)]
    pub indexing: bool,
    #[serde(default)]
    pub message: Option<String>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct GraphResultItem {
    pub name: String,
    pub file_path: String,
    pub chunk_id: String,
    pub depth: u32,
    #[serde(default)]
    pub direction: Option<String>,
}

// --- Index Status ---

#[derive(Debug, Clone, Deserialize)]
pub struct IndexStatusResponse {
    pub exists: bool,
    pub collection_name: Option<String>,
    pub repo_id: Option<u64>,
    pub empty: bool,
}

// ---------------------------------------------------------------------------
// Trait
// ---------------------------------------------------------------------------

#[async_trait]
pub trait ContextEngineApi: Send + Sync {
    /// Check index status for the configured repo.
    async fn index_status(&self) -> Result<IndexStatusResponse, ContextEngineError>;

    /// Search code in the indexed repo.
    async fn search(
        &self,
        query: &str,
        strategy: Option<&str>,
        limit: Option<u32>,
    ) -> Result<SearchResponse, ContextEngineError>;

    /// Query the dependency/blast-radius graph.
    async fn graph_query(
        &self,
        query: Option<&str>,
        function_name: Option<&str>,
        file_path: Option<&str>,
        query_type: Option<&str>,
    ) -> Result<GraphQueryResponse, ContextEngineError>;
}

// ---------------------------------------------------------------------------
// HTTP client implementation
// ---------------------------------------------------------------------------

pub struct ContextEngineClient {
    config: ContextEngineConfig,
    http: reqwest::Client,
}

impl ContextEngineClient {
    pub fn new(config: ContextEngineConfig) -> Self {
        Self {
            config,
            http: HTTP_CLIENT.clone(),
        }
    }

    /// Apply auth headers (X-Auth-Token, X-USER-ID, X-Workspace-ID) to a request.
    fn apply_auth_headers(&self, req: reqwest::RequestBuilder) -> reqwest::RequestBuilder {
        let mut r = req;
        if !self.config.auth_token.is_empty() {
            r = r.header("X-Auth-Token", &self.config.auth_token);
        }
        if !self.config.user_id.is_empty() {
            r = r.header("X-USER-ID", &self.config.user_id);
        }
        r = r.header("X-Workspace-ID", self.config.workspace_id.to_string());
        r
    }

    /// Send a GET and map errors.
    async fn get(&self, url: &str) -> Result<reqwest::Response, ContextEngineError> {
        let mut req = self.http.get(url);
        req = self.apply_auth_headers(req);
        let resp = req
            .send()
            .await
            .map_err(|e| ContextEngineError::Unavailable(e.to_string()))?;

        let status = resp.status();
        if !status.is_success() {
            let body = resp.text().await.unwrap_or_default();
            return Err(ContextEngineError::HttpError {
                status: status.as_u16(),
                body,
            });
        }
        Ok(resp)
    }

    /// Send a POST with JSON body and map errors.
    async fn post_json<T: Serialize>(
        &self,
        url: &str,
        body: &T,
    ) -> Result<reqwest::Response, ContextEngineError> {
        let mut req = self.http.post(url).json(body);
        req = self.apply_auth_headers(req);
        let resp = req
            .send()
            .await
            .map_err(|e| ContextEngineError::Unavailable(e.to_string()))?;

        let status = resp.status();
        if !status.is_success() {
            let body = resp.text().await.unwrap_or_default();
            return Err(ContextEngineError::HttpError {
                status: status.as_u16(),
                body,
            });
        }
        Ok(resp)
    }

    /// Deserialize JSON response body.
    async fn parse_json<T: serde::de::DeserializeOwned>(
        resp: reqwest::Response,
    ) -> Result<T, ContextEngineError> {
        let text = resp
            .text()
            .await
            .map_err(|e| ContextEngineError::Unavailable(e.to_string()))?;
        serde_json::from_str(&text).map_err(|e| {
            // Truncate at a char boundary to avoid panicking on multi-byte UTF-8
            let mut end = text.len().min(200);
            while !text.is_char_boundary(end) {
                end -= 1;
            }
            ContextEngineError::DeserializeError(format!("{e}: {}", &text[..end]))
        })
    }
}

#[async_trait]
impl ContextEngineApi for ContextEngineClient {
    async fn index_status(&self) -> Result<IndexStatusResponse, ContextEngineError> {
        let url = format!(
            "{}/api/v1/index/status?repo_path={}&user_id={}&workspace_id={}&machine_id={}",
            self.config.base_url,
            urlencoding(self.config.repo_path.as_str()),
            urlencoding(self.config.user_id.as_str()),
            self.config.workspace_id,
            urlencoding(self.config.machine_id.as_str()),
        );
        let resp = self.get(&url).await?;
        Self::parse_json(resp).await
    }

    async fn search(
        &self,
        query: &str,
        strategy: Option<&str>,
        limit: Option<u32>,
    ) -> Result<SearchResponse, ContextEngineError> {
        let url = format!("{}/search", self.config.base_url);
        let body = SearchRequest {
            query: query.to_string(),
            repo_path: self.config.repo_path.clone(),
            strategy: strategy.map(|s| s.to_string()),
            limit,
            user_id: self.config.user_id.clone(),
            workspace_id: self.config.workspace_id,
            machine_id: self.config.machine_id.clone(),
        };
        let resp = self.post_json(&url, &body).await?;
        Self::parse_json(resp).await
    }

    async fn graph_query(
        &self,
        query: Option<&str>,
        function_name: Option<&str>,
        file_path: Option<&str>,
        query_type: Option<&str>,
    ) -> Result<GraphQueryResponse, ContextEngineError> {
        let url = format!("{}/graph/query", self.config.base_url);
        let body = GraphQueryRequest {
            query: query.map(String::from),
            function_name: function_name.map(String::from),
            file_path: file_path.map(String::from),
            repo_path: self.config.repo_path.clone(),
            query_type: query_type.map(String::from),
            user_id: self.config.user_id.clone(),
            workspace_id: self.config.workspace_id,
            machine_id: self.config.machine_id.clone(),
        };
        let resp = self.post_json(&url, &body).await?;
        Self::parse_json(resp).await
    }
}

/// RFC 3986 percent-encoding for query string values.
fn urlencoding(s: &str) -> String {
    utf8_percent_encode(s, NON_ALPHANUMERIC).to_string()
}

// ---------------------------------------------------------------------------
// Mock (NOT behind #[cfg(test)] — Tauri layer needs it)
// ---------------------------------------------------------------------------

pub struct MockContextEngine {
    pub search_response: SearchResponse,
    pub graph_response: GraphQueryResponse,
    pub index_status_response: IndexStatusResponse,
}

impl MockContextEngine {
    /// Mock that reports as indexed with empty results.
    pub fn indexed_empty() -> Self {
        Self {
            search_response: SearchResponse {
                query: String::new(),
                total: 0,
                results: vec![],
                indexing: false,
                message: None,
            },
            graph_response: GraphQueryResponse {
                total: 0,
                results: vec![],
                indexing: false,
                message: None,
            },
            index_status_response: IndexStatusResponse {
                exists: true,
                collection_name: Some("test_collection".to_string()),
                repo_id: Some(1),
                empty: false,
            },
        }
    }

    /// Mock that reports as not-yet-indexed (cold start).
    pub fn not_indexed() -> Self {
        Self {
            search_response: SearchResponse {
                query: String::new(),
                total: 0,
                results: vec![],
                indexing: true,
                message: Some("Repository is being indexed".to_string()),
            },
            graph_response: GraphQueryResponse {
                total: 0,
                results: vec![],
                indexing: true,
                message: Some("Repository is being indexed".to_string()),
            },
            index_status_response: IndexStatusResponse {
                exists: false,
                collection_name: None,
                repo_id: None,
                empty: true,
            },
        }
    }

    /// Mock with preset search results.
    pub fn with_search_results(results: Vec<SearchResultItem>) -> Self {
        let total = results.len() as u32;
        Self {
            search_response: SearchResponse {
                query: String::new(),
                total,
                results,
                indexing: false,
                message: None,
            },
            graph_response: GraphQueryResponse {
                total: 0,
                results: vec![],
                indexing: false,
                message: None,
            },
            index_status_response: IndexStatusResponse {
                exists: true,
                collection_name: Some("test_collection".to_string()),
                repo_id: Some(1),
                empty: false,
            },
        }
    }

    /// Mock with preset graph results.
    pub fn with_graph_results(results: Vec<GraphResultItem>) -> Self {
        let total = results.len() as u32;
        Self {
            search_response: SearchResponse {
                query: String::new(),
                total: 0,
                results: vec![],
                indexing: false,
                message: None,
            },
            graph_response: GraphQueryResponse {
                total,
                results,
                indexing: false,
                message: None,
            },
            index_status_response: IndexStatusResponse {
                exists: true,
                collection_name: Some("test_collection".to_string()),
                repo_id: Some(1),
                empty: false,
            },
        }
    }
}

#[async_trait]
impl ContextEngineApi for MockContextEngine {
    async fn index_status(&self) -> Result<IndexStatusResponse, ContextEngineError> {
        Ok(self.index_status_response.clone())
    }

    async fn search(
        &self,
        _query: &str,
        _strategy: Option<&str>,
        _limit: Option<u32>,
    ) -> Result<SearchResponse, ContextEngineError> {
        Ok(self.search_response.clone())
    }

    async fn graph_query(
        &self,
        _query: Option<&str>,
        _function_name: Option<&str>,
        _file_path: Option<&str>,
        _query_type: Option<&str>,
    ) -> Result<GraphQueryResponse, ContextEngineError> {
        Ok(self.graph_response.clone())
    }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_mock_indexed_empty() {
        let mock = MockContextEngine::indexed_empty();

        let status = mock.index_status().await.unwrap();
        assert!(status.exists);
        assert!(!status.empty);

        let search = mock.search("test", None, None).await.unwrap();
        assert_eq!(search.total, 0);
        assert!(search.results.is_empty());
        assert!(!search.indexing);
    }

    #[tokio::test]
    async fn test_mock_not_indexed() {
        let mock = MockContextEngine::not_indexed();

        let status = mock.index_status().await.unwrap();
        assert!(!status.exists);

        let search = mock.search("test", None, None).await.unwrap();
        assert!(search.indexing);
        assert!(search.message.is_some());
    }

    #[tokio::test]
    async fn test_mock_with_search_results() {
        let results = vec![
            SearchResultItem {
                chunk_id: "c1".into(),
                content: "fn main() {}".into(),
                file_path: "src/main.rs".into(),
                language: "rust".into(),
                score: 0.95,
                source: "vector".into(),
            },
            SearchResultItem {
                chunk_id: "c2".into(),
                content: "fn helper() {}".into(),
                file_path: "src/lib.rs".into(),
                language: "rust".into(),
                score: 0.80,
                source: "keyword".into(),
            },
        ];
        let mock = MockContextEngine::with_search_results(results);

        let resp = mock.search("main", None, None).await.unwrap();
        assert_eq!(resp.total, 2);
        assert_eq!(resp.results.len(), 2);
        assert_eq!(resp.results[0].file_path, "src/main.rs");
        assert!((resp.results[0].score - 0.95).abs() < f32::EPSILON);
    }

    #[tokio::test]
    async fn test_mock_with_graph_results() {
        let results = vec![GraphResultItem {
            name: "process_request".into(),
            file_path: "src/handler.rs".into(),
            chunk_id: "g1".into(),
            depth: 1,
            direction: Some("caller".into()),
        }];
        let mock = MockContextEngine::with_graph_results(results);

        let resp = mock
            .graph_query(None, Some("process_request"), None, Some("blast_radius"))
            .await
            .unwrap();
        assert_eq!(resp.total, 1);
        assert_eq!(resp.results[0].name, "process_request");
        assert_eq!(resp.results[0].direction.as_deref(), Some("caller"));
    }

    #[test]
    fn test_search_request_serialization() {
        let req = SearchRequest {
            query: "find auth".into(),
            repo_path: "/home/user/repo".into(),
            strategy: Some("multi".into()),
            limit: Some(10),
            user_id: "u1".into(),
            workspace_id: 42,
            machine_id: "m1".into(),
        };
        let json = serde_json::to_value(&req).unwrap();
        assert_eq!(json["query"], "find auth");
        assert_eq!(json["repo_path"], "/home/user/repo");
        assert_eq!(json["strategy"], "multi");
        assert_eq!(json["limit"], 10);
        assert_eq!(json["user_id"], "u1");
        assert_eq!(json["workspace_id"], 42);
        assert_eq!(json["machine_id"], "m1");
    }

    #[test]
    fn test_graph_request_serialization() {
        let req = GraphQueryRequest {
            query: None,
            function_name: Some("do_stuff".into()),
            file_path: None,
            repo_path: "/repo".into(),
            query_type: Some("blast_radius".into()),
            user_id: "u1".into(),
            workspace_id: 1,
            machine_id: "m1".into(),
        };
        let json = serde_json::to_value(&req).unwrap();
        // None fields must be omitted
        assert!(json.get("query").is_none());
        assert!(json.get("file_path").is_none());
        // Present fields
        assert_eq!(json["function_name"], "do_stuff");
        assert_eq!(json["query_type"], "blast_radius");
        assert_eq!(json["repo_path"], "/repo");
    }

    #[test]
    fn test_search_response_deserialization() {
        let json = r#"{
            "query": "auth handler",
            "total": 1,
            "results": [{
                "chunk_id": "abc123",
                "content": "pub fn authenticate() { ... }",
                "file_path": "src/auth.rs",
                "language": "rust",
                "score": 0.92,
                "source": "hybrid"
            }],
            "indexing": false
        }"#;
        let resp: SearchResponse = serde_json::from_str(json).unwrap();
        assert_eq!(resp.query, "auth handler");
        assert_eq!(resp.total, 1);
        assert_eq!(resp.results[0].chunk_id, "abc123");
        assert!((resp.results[0].score - 0.92).abs() < f32::EPSILON);
        assert_eq!(resp.results[0].source, "hybrid");
        assert!(!resp.indexing);
        assert!(resp.message.is_none());
    }

    #[test]
    fn test_graph_response_deserialization() {
        let json = r#"{
            "total": 2,
            "results": [
                {
                    "name": "handle_request",
                    "file_path": "src/server.rs",
                    "chunk_id": "g1",
                    "depth": 0
                },
                {
                    "name": "parse_body",
                    "file_path": "src/parser.rs",
                    "chunk_id": "g2",
                    "depth": 1,
                    "direction": "dependency"
                }
            ]
        }"#;
        let resp: GraphQueryResponse = serde_json::from_str(json).unwrap();
        assert_eq!(resp.total, 2);
        assert_eq!(resp.results[0].name, "handle_request");
        assert!(resp.results[0].direction.is_none());
        assert_eq!(resp.results[1].direction.as_deref(), Some("dependency"));
        assert!(!resp.indexing);
    }

    #[test]
    fn test_index_status_deserialization() {
        let json = r#"{
            "exists": true,
            "collection_name": "repo_abc",
            "repo_id": 42,
            "empty": false
        }"#;
        let resp: IndexStatusResponse = serde_json::from_str(json).unwrap();
        assert!(resp.exists);
        assert_eq!(resp.collection_name.as_deref(), Some("repo_abc"));
        assert_eq!(resp.repo_id, Some(42));
        assert!(!resp.empty);

        // Also test minimal response (no optional fields)
        let json_minimal = r#"{"exists": false, "empty": true}"#;
        let resp2: IndexStatusResponse = serde_json::from_str(json_minimal).unwrap();
        assert!(!resp2.exists);
        assert!(resp2.collection_name.is_none());
        assert!(resp2.repo_id.is_none());
        assert!(resp2.empty);
    }

    #[test]
    fn test_urlencoding() {
        // NON_ALPHANUMERIC encodes everything except [A-Za-z0-9]
        // Spaces
        assert_eq!(urlencoding("hello world"), "hello%20world");
        // Special query-string characters
        assert_eq!(urlencoding("a&b=c"), "a%26b%3Dc");
        assert_eq!(urlencoding("a?b#c"), "a%3Fb%23c");
        // Percent
        assert_eq!(urlencoding("100%"), "100%25");
        // Plus sign
        assert_eq!(urlencoding("a+b"), "a%2Bb");
        // Backslash + colon (Windows paths)
        assert_eq!(urlencoding(r"C:\Users\foo"), "C%3A%5CUsers%5Cfoo");
        // Forward slashes are encoded (NON_ALPHANUMERIC)
        assert_eq!(urlencoding("/home/user/repo"), "%2Fhome%2Fuser%2Frepo");
        // Brackets — the bug that prompted this fix
        assert_eq!(urlencoding("path[0]"), "path%5B0%5D");
        assert_eq!(urlencoding("path{a}"), "path%7Ba%7D");
        // No-op on clean alphanumeric strings
        assert_eq!(urlencoding("simple"), "simple");
    }

    #[test]
    fn test_search_request_none_fields_omitted() {
        let req = SearchRequest {
            query: "test".into(),
            repo_path: "/repo".into(),
            strategy: None,
            limit: None,
            user_id: "u1".into(),
            workspace_id: 1,
            machine_id: "m1".into(),
        };
        let json = serde_json::to_value(&req).unwrap();
        assert!(json.get("strategy").is_none(), "None strategy must be omitted");
        assert!(json.get("limit").is_none(), "None limit must be omitted");
        // Required fields still present
        assert_eq!(json["query"], "test");
    }

    #[test]
    fn test_deserialize_error_truncation_utf8_safe() {
        // Build a string where byte offset 200 falls inside a multi-byte char.
        // '€' is 3 bytes (E2 82 AC). Fill 198 ASCII bytes + '€' = 201 bytes.
        let mut body = "x".repeat(198);
        body.push('€'); // bytes 198..201
        assert_eq!(body.len(), 201);

        // Simulate what parse_json does on deser failure
        let mut end = body.len().min(200);
        while !body.is_char_boundary(end) {
            end -= 1;
        }
        let snippet = &body[..end];
        // Should truncate to 198 (before the €), not panic
        assert_eq!(snippet.len(), 198);
        assert!(snippet.ends_with('x'));
    }
}
