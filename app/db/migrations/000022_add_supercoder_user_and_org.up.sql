INSERT INTO organisations (id, name) VALUES (1, 'SuperCoderOrg');
INSERT INTO users (id, name, email, password, organisation_id, created_at, updated_at)
VALUES (1, 'SuperCoderUser', 'supercoder@superagi.com', 'password', 1, NOW(), NOW());