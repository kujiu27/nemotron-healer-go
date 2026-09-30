"""Unit tests enforcing model validation, serialization, and ORM conversion."""

import pytest
from models import Item, Order


def test_item_validation_success():
    item = Item(id=1, name="GPU Accelerator", price=999.99, tags=["nvidia", "ai"])
    assert item.id == 1
    assert item.price == 999.99
    assert "nvidia" in item.tags


def test_item_price_negative_fails():
    with pytest.raises(ValueError):
        Item(id=2, name="Invalid Item", price=-10.0)


def test_order_from_raw():
    payload = {
        "order_id": "ORD-2026-X",
        "items": [
            {"id": 10, "name": "Nemotron-3-Compute", "price": 450.0}
        ],
        "total": 450.0
    }
    order = Order.from_raw(payload)
    assert order.order_id == "ORD-2026-X"
    assert len(order.items) == 1
    assert order.items[0].name == "Nemotron-3-Compute"


def test_order_to_dict():
    payload = {
        "order_id": "ORD-2026-Y",
        "items": [
            {"id": 20, "name": "Tavily Search Unit", "price": 50.0}
        ],
        "total": 50.0
    }
    order = Order.from_raw(payload)
    d = order.to_dict()
    assert isinstance(d, dict)
    assert d["order_id"] == "ORD-2026-Y"
    assert d["items"][0]["price"] == 50.0
