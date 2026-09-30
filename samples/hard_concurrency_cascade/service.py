"""High-level banking facade coordinating ledger engine and multi-step transfers."""

import asyncio
from typing import List
from engine import LedgerEngine


class BankingService:
    def __init__(self, initial_balance: float = 2000.0):
        self.engine = LedgerEngine(initial_balance=initial_balance)

    async def single_transfer(self, account_id: str, amount: float):
        return await self.engine.mutate_balance(account_id, -amount)

    async def atomic_batch_transfer(self, account_id: str, amounts: List[float]):
        """Batch transfer must be atomic. It holds the transaction lock across all sub-transfers."""
        async with self.engine.lock:
            total_deducted = 0.0
            for amt in amounts:
                await self.engine.mutate_balance(account_id, -amt)
                total_deducted += amt
            return total_deducted
