use rusqlite::Connection;

/// Create the `watched_repos` table if it doesn't exist.
pub fn ensure_watched_repos_table(conn: &Connection) -> rusqlite::Result<()> {
    conn.execute_batch(
        "CREATE TABLE IF NOT EXISTS watched_repos (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            repo_path TEXT NOT NULL UNIQUE,
            last_used_at TEXT NOT NULL,
            created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
        );",
    )
}

// NOTE: chat-desktop's `get_or_create_machine_id` was intentionally dropped.
// SuperCoder has a single machine_id source — `commands::machine_id(app_state)`
// (settings key "machine_id"). The watcher reuses that exact value so its
// context-engine collection key matches the search client's.

/// Upsert a repo as actively watched (updates `last_used_at`).
pub fn upsert_watched_repo(conn: &Connection, repo_path: &str) -> rusqlite::Result<()> {
    conn.execute(
        "INSERT INTO watched_repos (repo_path, last_used_at)
         VALUES (?1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
         ON CONFLICT(repo_path) DO UPDATE SET last_used_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')",
        [repo_path],
    )?;
    Ok(())
}

/// Get repos watched within the last 7 days.
pub fn get_active_watched_repos(conn: &Connection) -> rusqlite::Result<Vec<String>> {
    let mut stmt = conn.prepare(
        "SELECT repo_path FROM watched_repos
         WHERE last_used_at > strftime('%Y-%m-%dT%H:%M:%fZ', 'now', '-7 days')",
    )?;
    let rows = stmt.query_map([], |row| row.get(0))?;
    let mut repos = Vec::new();
    for row in rows {
        repos.push(row?);
    }
    Ok(repos)
}

/// Delete repos older than 7 days. Returns number of rows deleted.
pub fn cleanup_stale_repos(conn: &Connection) -> rusqlite::Result<usize> {
    let count = conn.execute(
        "DELETE FROM watched_repos
         WHERE last_used_at <= strftime('%Y-%m-%dT%H:%M:%fZ', 'now', '-7 days')",
        [],
    )?;
    Ok(count)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn setup_db() -> Connection {
        let conn = Connection::open_in_memory().unwrap();
        conn.execute_batch(
            "CREATE TABLE IF NOT EXISTS settings (
                key TEXT PRIMARY KEY,
                value TEXT NOT NULL,
                updated_at TEXT DEFAULT CURRENT_TIMESTAMP
            );",
        )
        .unwrap();
        ensure_watched_repos_table(&conn).unwrap();
        conn
    }

    #[test]
    fn test_upsert_and_get_repos() {
        let conn = setup_db();
        upsert_watched_repo(&conn, "/home/user/project-a").unwrap();
        upsert_watched_repo(&conn, "/home/user/project-b").unwrap();

        let repos = get_active_watched_repos(&conn).unwrap();
        assert_eq!(repos.len(), 2);
        assert!(repos.contains(&"/home/user/project-a".to_string()));
        assert!(repos.contains(&"/home/user/project-b".to_string()));
    }

    #[test]
    fn test_stale_repo_excluded() {
        let conn = setup_db();

        // Insert a repo, then manually set its last_used_at to 8 days ago
        upsert_watched_repo(&conn, "/home/user/old-project").unwrap();
        conn.execute(
            "UPDATE watched_repos SET last_used_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now', '-8 days')
             WHERE repo_path = '/home/user/old-project'",
            [],
        )
        .unwrap();

        // Insert a fresh repo
        upsert_watched_repo(&conn, "/home/user/fresh-project").unwrap();

        let repos = get_active_watched_repos(&conn).unwrap();
        assert_eq!(repos.len(), 1);
        assert_eq!(repos[0], "/home/user/fresh-project");
    }

    #[test]
    fn test_cleanup_removes_stale() {
        let conn = setup_db();

        upsert_watched_repo(&conn, "/home/user/old").unwrap();
        conn.execute(
            "UPDATE watched_repos SET last_used_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now', '-8 days')
             WHERE repo_path = '/home/user/old'",
            [],
        )
        .unwrap();

        upsert_watched_repo(&conn, "/home/user/recent").unwrap();

        let deleted = cleanup_stale_repos(&conn).unwrap();
        assert_eq!(deleted, 1);

        let repos = get_active_watched_repos(&conn).unwrap();
        assert_eq!(repos.len(), 1);
        assert_eq!(repos[0], "/home/user/recent");
    }
}
