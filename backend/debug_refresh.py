#!/usr/bin/env python
"""Debug script to test refresh token functionality"""

import sys
import os
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from fastapi.testclient import TestClient
from nexorious.main import app
from nexorious.tests.integration_test_utils import create_test_user_data
from nexorious.core.security import verify_token, hash_token
from nexorious.core.database import get_session
from nexorious.models.user import UserSession
from sqlmodel import select
from datetime import datetime, timezone

client = TestClient(app)
user_data = create_test_user_data()

# Register user
register_response = client.post('/api/auth/register', json=user_data)
print('Register response:', register_response.status_code)

# Login user
login_response = client.post('/api/auth/login', json={
    'username': user_data['email'],
    'password': user_data['password']
})
print('Login response:', login_response.status_code)

if login_response.status_code == 200:
    login_data = login_response.json()
    refresh_token = login_data['refresh_token']
    
    print('Testing refresh token verification...')
    
    # Test token verification
    try:
        payload = verify_token(refresh_token, token_type="refresh")
        print('Token verification successful. Payload:', payload)
        
        user_id = payload.get("sub")
        print('User ID from token:', user_id)
        
        # Check database session
        from nexorious.core.database import engine
        from sqlmodel import Session
        
        with Session(engine) as db_session:
            refresh_token_hash = hash_token(refresh_token)
            print('Refresh token hash:', refresh_token_hash[:20] + '...')
            
            session_record = db_session.exec(
                select(UserSession).where(
                    (UserSession.user_id == user_id) & 
                    (UserSession.refresh_token_hash == refresh_token_hash) &
                    (UserSession.expires_at > datetime.now(timezone.utc))
                )
            ).first()
            
            print('Session record found:', session_record is not None)
            
            if session_record:
                print('Session ID:', session_record.id)
                print('Session expires at:', session_record.expires_at)
                print('Current time:', datetime.now(timezone.utc))
                
            # Check all sessions for this user
            all_sessions = db_session.exec(
                select(UserSession).where(UserSession.user_id == user_id)
            ).all()
            
            print('All sessions for user:', len(all_sessions))
            for session in all_sessions:
                print(f'  Session {session.id}: expires {session.expires_at}')
                print(f'    Token hash matches: {session.refresh_token_hash == refresh_token_hash}')
        
    except Exception as e:
        print('Token verification failed:', str(e))
        import traceback
        traceback.print_exc()

else:
    print('Login failed:', login_response.json())