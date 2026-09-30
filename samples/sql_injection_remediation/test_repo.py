"""Tests verifying SQL injection neutralization and safe parameterized queries."""

import pytest
from repo import UserRepository


def test_normal_lookup():
    repo = UserRepository()
    user = repo.get_user_by_username("alice")
    assert user is not None
    assert user["role"] == "developer"


def test_sql_injection_attack_must_be_neutralized():
    repo = UserRepository()
    # Classic SQL injection bypass: ' OR '1'='1
    malicious_input = "' OR '1'='1"
    user = repo.get_user_by_username(malicious_input)

    # In a secure parameterized query, no user has literal username "' OR '1'='1"
    # A vulnerable query would return the first user ('admin')
    assert user is None, f"Security vulnerability! SQL Injection succeeded: {user}"
