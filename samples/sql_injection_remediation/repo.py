"""User repository with insecure SQL string interpolation."""

import sqlite3
from typing import Optional, Dict


class UserRepository:
    def __init__(self, db_path: str = ":memory:"):
        self.conn = sqlite3.connect(db_path)
        self._init_db()

    def _init_db(self):
        with self.conn:
            self.conn.execute("CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT, role TEXT)")
            self.conn.execute("INSERT INTO users (username, role) VALUES ('admin', 'superadmin')")
            self.conn.execute("INSERT INTO users (username, role) VALUES ('alice', 'developer')")

    def get_user_by_username(self, username: str) -> Optional[Dict]:
        # VULNERABLE: Direct string formatting vulnerable to SQL Injection
        query = f"SELECT id, username, role FROM users WHERE username = '{username}'"
        cursor = self.conn.cursor()
        cursor.execute(query)
        row = cursor.fetchone()
        if row:
            return {"id": row[0], "username": row[1], "role": row[2]}
        return None
