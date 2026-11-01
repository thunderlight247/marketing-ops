-- Migration 0016: Team Role-Based Access Control (RBAC) on operators
ALTER TABLE operators
    ADD COLUMN IF NOT EXISTS role text NOT NULL DEFAULT 'admin' CHECK (role IN ('admin', 'editor', 'viewer')),
    ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive'));
