-- +migrate Down
ALTER TABLE projects DROP COLUMN backend_url;
ALTER TABLE projects DROP COLUMN frontend_url;
ALTER TABLE projects DROP COLUMN url;
ALTER TABLE projects DROP COLUMN hash_id;