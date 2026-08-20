CREATE TABLE plugin_active_versions (
    name TEXT PRIMARY KEY REFERENCES plugins(name) ON DELETE CASCADE,
    version TEXT NOT NULL
);

INSERT INTO plugin_active_versions(name, version)
SELECT name, version FROM plugins WHERE version <> '';
