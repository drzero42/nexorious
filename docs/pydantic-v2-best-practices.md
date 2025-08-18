# Pydantic V2 Best Practices

Quick reference for writing optimal Pydantic V2 code.

## Key V2 Method Changes

```python
# V1 → V2 Method Migrations
model.dict() → model.model_dump()
model.json() → model.model_dump_json()
Model.parse_obj(data) → Model.model_validate(data)
Model.parse_raw(json_str) → Model.model_validate_json(json_str)
Model.from_orm(orm_obj) → Model.model_validate(orm_obj, from_attributes=True)
```

## Modern Validator Patterns

### Field Validators (Preferred)
```python
from pydantic import BaseModel, field_validator

class User(BaseModel):
    email: str
    age: int
    
    @field_validator('email')
    @classmethod
    def validate_email(cls, v: str) -> str:
        if '@' not in v:
            raise ValueError('Invalid email')
        return v.lower()
    
    @field_validator('age')
    @classmethod
    def validate_age(cls, v: int) -> int:
        if v < 0 or v > 150:
            raise ValueError('Age must be 0-150')
        return v
```

### Annotated Validators (Most Reusable)
```python
from typing import Annotated
from pydantic import BaseModel, AfterValidator, BeforeValidator

def normalize_email(v: str) -> str:
    return v.lower().strip()

def validate_positive(v: int) -> int:
    if v <= 0:
        raise ValueError('Must be positive')
    return v

Email = Annotated[str, BeforeValidator(normalize_email)]
PositiveInt = Annotated[int, AfterValidator(validate_positive)]

class User(BaseModel):
    email: Email
    score: PositiveInt
```

### Model Validators (Cross-Field Validation)
```python
from pydantic import BaseModel, model_validator

class User(BaseModel):
    password: str
    confirm_password: str
    
    @model_validator(mode='after')
    def check_passwords_match(self):
        if self.password != self.confirm_password:
            raise ValueError('Passwords do not match')
        return self
```

## Configuration Best Practices

```python
from pydantic import BaseModel, ConfigDict

class User(BaseModel):
    model_config = ConfigDict(
        str_strip_whitespace=True,
        validate_assignment=True,
        extra='forbid',
        from_attributes=True  # For ORM objects
    )
    
    name: str
    email: str
```

## Type Constraints with Annotated

```python
from typing import Annotated
from pydantic import BaseModel, Field, StringConstraints

class User(BaseModel):
    name: Annotated[str, StringConstraints(min_length=1, max_length=100)]
    age: Annotated[int, Field(ge=0, le=150)]
    email: Annotated[str, Field(pattern=r'^[^@]+@[^@]+\.[^@]+$')]
```

## Serialization Control

```python
from pydantic import BaseModel, Field, field_serializer

class User(BaseModel):
    password: str = Field(exclude=True)  # Never serialize
    created_at: datetime
    
    @field_serializer('created_at')
    def serialize_datetime(self, value: datetime) -> str:
        return value.isoformat()
```

## Common Patterns

### Optional Fields with Defaults
```python
from typing import Optional
from pydantic import BaseModel, Field

class User(BaseModel):
    name: str
    email: Optional[str] = None
    active: bool = Field(default=True)
    metadata: dict = Field(default_factory=dict)
```

### Union Types (Preserve Input Type)
```python
from typing import Union
from pydantic import BaseModel

class Response(BaseModel):
    data: Union[str, int, dict]  # V2 preserves original type
```

### Custom Root Models
```python
from typing import List
from pydantic import BaseModel, RootModel

class UserList(RootModel[List[User]]):
    root: List[User]
    
    def __iter__(self):
        return iter(self.root)
    
    def __len__(self):
        return len(self.root)
```

## Performance Tips

1. Use `TypeAdapter` for non-BaseModel validation
2. Set `model_config = ConfigDict(extra='forbid')` when possible
3. Use `Annotated` types for reusability
4. Leverage strict mode for better performance: `validate_call(strict=True)`
5. Cache validators when processing large datasets

## Migration Checklist

- [ ] Update method names (`dict()` → `model_dump()`)
- [ ] Replace `@validator` with `@field_validator`
- [ ] Replace `@root_validator` with `@model_validator`
- [ ] Update `Config` class to `model_config` dict
- [ ] Review union type handling
- [ ] Update serialization patterns
- [ ] Test model equality comparisons