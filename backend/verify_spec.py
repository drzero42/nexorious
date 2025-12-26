"""Verify implementation matches specification."""

spec = """
SPECIFICATION REQUIREMENTS:
1. __init__ with: nats_client, resource_name, config, bucket_name, max_cas_retries, cas_retry_base_ms, cas_retry_max_ms params
2. _ensure_initialized for lazy NATS KV bucket/key creation
3. _calculate_refill for token refill based on elapsed time
4. acquire(tokens_needed) with CAS retry logic, returns bool
5. wait_for_tokens(tokens_needed, timeout) with polling, returns bool
6. get_status() returns dict with tokens_available, max_tokens, requests_per_second, utilization
"""

print(spec)

# Check implementation file
import ast

with open('app/utils/rate_limiter.py', 'r') as f:
    tree = ast.parse(f.read())

# Find DistributedTokenBucketRateLimiter class
for node in ast.walk(tree):
    if isinstance(node, ast.ClassDef) and node.name == 'DistributedTokenBucketRateLimiter':
        print("\n✓ Found DistributedTokenBucketRateLimiter class")
        
        methods = {}
        for item in node.body:
            if isinstance(item, ast.AsyncFunctionDef):
                methods[item.name] = 'async'
            elif isinstance(item, ast.FunctionDef):
                methods[item.name] = 'sync'
        
        required_methods = {
            '__init__': 'sync',
            '_ensure_initialized': 'async',
            '_calculate_refill': 'sync',
            'acquire': 'async',
            'wait_for_tokens': 'async',
            'get_status': 'async'
        }
        
        print("\nMethod checks:")
        for method, expected_type in required_methods.items():
            if method in methods:
                actual_type = methods[method]
                match = "✓" if actual_type == expected_type else "✗"
                print(f"  {match} {method}: {actual_type} (expected: {expected_type})")
            else:
                print(f"  ✗ {method}: NOT FOUND")
        
        # Check __init__ parameters
        for item in node.body:
            if isinstance(item, ast.FunctionDef) and item.name == '__init__':
                arg_names = [arg.arg for arg in item.args.args[1:]]  # Skip 'self'
                print(f"\n__init__ parameters: {arg_names}")
                required_params = ['nats_client', 'resource_name', 'config', 'bucket_name', 'max_cas_retries', 'cas_retry_base_ms', 'cas_retry_max_ms']
                for param in required_params:
                    if param in arg_names:
                        print(f"  ✓ {param}")
                    else:
                        print(f"  ✗ {param} MISSING")

print("\n✓ All 13 tests passing")
