"""Integration test suite for cascading concurrency and deadlock detection."""

import asyncio
import pytest
from service import BankingService


def test_concurrent_single_transfers_must_be_atomic():
    """Test 1: 10 concurrent single transfers of 100.0 must leave balance at 1000.0 (Starting at 2000.0)."""
    async def _run():
        srv = BankingService(initial_balance=2000.0)
        tasks = [srv.single_transfer("acc_1", 100.0) for _ in range(10)]
        await asyncio.gather(*tasks)
        assert srv.engine.balance == 1000.0, f"Race condition detected! Expected 1000.0, got {srv.engine.balance}"

    asyncio.run(_run())


def test_atomic_batch_transfer_must_not_deadlock():
    """Test 2: Batch transfer acquires parent lock. If sub-call is not re-entrant, this DEADLOCKS!"""
    async def _run():
        srv = BankingService(initial_balance=2000.0)
        # Timeout after 0.5 seconds: if lock is non-reentrant, it will hang and fail timeout!
        try:
            total = await asyncio.wait_for(srv.atomic_batch_transfer("acc_2", [50.0, 50.0, 100.0]), timeout=0.5)
            assert total == 200.0
            assert srv.engine.balance == 1800.0
        except asyncio.TimeoutError:
            pytest.fail("FATAL DEADLOCK: atomic_batch_transfer hung due to non-reentrant lock deadlock!")

    asyncio.run(_run())


def test_mixed_concurrency_and_audit_integrity():
    """Test 3: Mixed concurrent batches and single transfers must preserve balance and audit count."""
    async def _run():
        srv = BankingService(initial_balance=3000.0)
        # 5 singles (-100 each = -500) and 2 batches (-200 each = -400) -> Balance should be 2100.0
        singles = [srv.single_transfer("acc_mixed", 100.0) for _ in range(5)]
        batches = [srv.atomic_batch_transfer("acc_mixed", [100.0, 100.0]) for _ in range(2)]
        await asyncio.gather(*singles, *batches)
        assert srv.engine.balance == 2100.0, f"Expected 2100.0, got {srv.engine.balance}"
        assert srv.engine.auditor.count() == 9, f"Expected 9 audit records, got {srv.engine.auditor.count()}"

    asyncio.run(_run())
