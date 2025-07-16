#!/usr/bin/env python
"""Simple test to verify refresh token functionality"""

import pytest
from fastapi.testclient import TestClient
from sqlmodel import Session, SQLModel, create_engine
from sqlmodel.pool import StaticPool

from nexorious.main import app
from nexorious.core.database import get_session
from nexorious.tests.integration_test_utils import create_test_user_data

# Create test database session
engine = create_engine(
    "sqlite:///:memory:",
    connect_args={"check_same_thread": False},
    poolclass=StaticPool,
)
SQLModel.metadata.create_all(engine)

def get_test_session():
    with Session(engine) as session:
        yield session

# Override the dependency
app.dependency_overrides[get_session] = get_test_session

client = TestClient(app)

# Test the flow
user_data = create_test_user_data()
print("User data:", user_data)

# Register user
register_response = client.post('/api/auth/register', json=user_data)
print('Register response:', register_response.status_code)
if register_response.status_code != 201:
    print('Register error:', register_response.json())
    exit(1)

# Login user
login_response = client.post('/api/auth/login', json={
    'username': user_data['email'],
    'password': user_data['password']
})
print('Login response:', login_response.status_code)
if login_response.status_code != 200:
    print('Login error:', login_response.json())
    exit(1)

login_data = login_response.json()
refresh_token = login_data['refresh_token']

# Try refresh
refresh_response = client.post('/api/auth/refresh', json={'refresh_token': refresh_token})
print('Refresh response:', refresh_response.status_code)
print('Refresh data:', refresh_response.json())

# Clean up
app.dependency_overrides.clear()