"""Tests for atomic concurrency on BankLedger."""

import asyncio
import pytest
from account import BankLedger


def test_concurrent_transfers_must_be_atomic():
    async def _run():
        ledger = BankLedger(initial_balance=1000.0)
        # 10 concurrent transfers of 50.0 should leave balance at 500.0
        tasks = [ledger.transfer(50.0) for _ in range(10)]
        await asyncio.gather(*tasks)
        assert ledger.balance == 500.0, f"Balance corrupted! Expected 500.0, got {ledger.balance}"

    asyncio.run(_run())
