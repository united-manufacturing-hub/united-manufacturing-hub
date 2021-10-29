# =============================================================================
# Generated using -> Source: app.quicktype.io
# 
# To use this code, make sure you
# 
#     import json
# 
# and then, to convert JSON from a string, do
# 
#     result = product_image_from_dict(json.loads(json_string))
# =============================================================================

from dataclasses import dataclass
from typing import Any, TypeVar, Type, cast


T = TypeVar("T")


def from_str(x: Any) -> str:
    assert isinstance(x, str)
    return x


def from_int(x: Any) -> int:
    assert isinstance(x, int) and not isinstance(x, bool)
    return x


def to_class(c: Type[T], x: Any) -> dict:
    assert isinstance(x, c)
    return cast(Any, x).to_dict()


@dataclass
class Image:
    image_id: str
    image_bytes: str
    image_height: int
    image_width: int
    image_channels: int

    @staticmethod
    def from_dict(obj: Any) -> 'Image':
        assert isinstance(obj, dict)
        image_id = from_str(obj.get("image_id"))
        image_bytes = from_str(obj.get("image_bytes"))
        image_height = from_int(obj.get("image_height"))
        image_width = from_int(obj.get("image_width"))
        image_channels = from_int(obj.get("image_channels"))
        return Image(image_id, image_bytes, image_height, image_width, image_channels)

    def to_dict(self) -> dict:
        result: dict = {}
        result["image_id"] = from_str(self.image_id)
        result["image_bytes"] = from_str(self.image_bytes)
        result["image_height"] = from_int(self.image_height)
        result["image_width"] = from_int(self.image_width)
        result["image_channels"] = from_int(self.image_channels)
        return result


@dataclass
class ProductImage:
    timestamp_ms: int
    image: Image

    @staticmethod
    def from_dict(obj: Any) -> 'ProductImage':
        assert isinstance(obj, dict)
        timestamp_ms = from_int(obj.get("timestamp_ms"))
        image = Image.from_dict(obj.get("image"))
        return ProductImage(timestamp_ms, image)

    def to_dict(self) -> dict:
        result: dict = {}
        result["timestamp_ms"] = from_int(self.timestamp_ms)
        result["image"] = to_class(Image, self.image)
        return result


def product_image_from_dict(s: Any) -> ProductImage:
    return ProductImage.from_dict(s)


def product_image_to_dict(x: ProductImage) -> Any:
    return to_class(ProductImage, x)
