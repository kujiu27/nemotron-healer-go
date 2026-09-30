"""Sample application models written with legacy Pydantic v1 syntax."""

from typing import List, Optional
from pydantic import BaseModel, ConfigDict, field_validator


class Item(BaseModel):
    id: int
    name: str
    price: float
    tags: List[str] = []

    model_config = ConfigDict(from_attributes=True)

    @field_validator("price")
    @classmethod
    def check_price(cls, v):
        if v <= 0:
            raise ValueError("Price must be positive")
        return v


class Order(BaseModel):
    order_id: str
    items: List[Item]
    total: float = 0.0

    @classmethod
    def from_raw(cls, data: dict):
        return cls.model_validate(data)

    def to_dict(self) -> dict:
        return self.model_dump()