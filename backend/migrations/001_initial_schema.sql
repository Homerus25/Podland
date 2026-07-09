-- Users table
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Machines table
CREATE TABLE IF NOT EXISTS machines (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    description TEXT,
    hostname TEXT NOT NULL,
    port INTEGER DEFAULT 22,
    ssh_username TEXT NOT NULL,
    ssh_key_encrypted BLOB NOT NULL,
    ssh_key_fingerprint TEXT,
    podman_socket_path TEXT DEFAULT '/run/podman/podman.sock',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Machine access permissions
CREATE TABLE IF NOT EXISTS machine_access (
    machine_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    permission TEXT NOT NULL CHECK(permission IN ('read', 'write', 'admin')),
    granted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (machine_id, user_id),
    FOREIGN KEY (machine_id) REFERENCES machines(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_machines_owner_id ON machines(owner_id);
CREATE INDEX IF NOT EXISTS idx_machines_name ON machines(name);
CREATE INDEX IF NOT EXISTS idx_machine_access_machine_id ON machine_access(machine_id);
CREATE INDEX IF NOT EXISTS idx_machine_access_user_id ON machine_access(user_id);

-- Create triggers for updated_at
CREATE TRIGGER IF NOT EXISTS update_machine_updated_at
AFTER UPDATE ON machines
FOR EACH ROW
BEGIN
    UPDATE machines SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id;
END;
