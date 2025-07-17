#!/usr/bin/env python3
"""
Simple integration test for manim-mcp-server
"""
import os
import sys
import subprocess
import tempfile

def test_manim_available():
    """Test that manim command is available"""
    try:
        result = subprocess.run(['manim', '--version'], capture_output=True, text=True)
        if result.returncode == 0:
            print(f"✓ Manim available: {result.stdout.strip()}")
            return True
        else:
            print(f"✗ Manim command failed: {result.stderr}")
            return False
    except FileNotFoundError:
        print("✗ Manim command not found in PATH")
        return False

def test_mcp_import():
    """Test that MCP can be imported"""
    try:
        import mcp.server.fastmcp
        print("✓ MCP import successful")
        return True
    except ImportError as e:
        print(f"✗ MCP import failed: {e}")
        return False

def test_manim_simple_render():
    """Test a simple manim render"""
    simple_scene = '''
from manim import *

class SimpleTest(Scene):
    def construct(self):
        text = Text("Hello, Manim!")
        self.add(text)
'''
    
    try:
        with tempfile.NamedTemporaryFile(mode='w', suffix='.py', delete=False) as f:
            f.write(simple_scene)
            temp_file = f.name
        
        # Test manim render
        result = subprocess.run([
            'manim', '-ql', '--disable_caching', temp_file, 'SimpleTest'
        ], capture_output=True, text=True, cwd=tempfile.gettempdir())
        
        os.unlink(temp_file)
        
        if result.returncode == 0:
            print("✓ Simple manim render successful")
            return True
        else:
            print(f"✗ Manim render failed: {result.stderr}")
            return False
    except Exception as e:
        print(f"✗ Manim render test error: {e}")
        return False

def main():
    print("Testing manim-mcp-server integration...")
    print("=" * 50)
    
    tests = [
        test_manim_available,
        test_mcp_import, 
        test_manim_simple_render
    ]
    
    results = []
    for test in tests:
        results.append(test())
        print()
    
    passed = sum(results)
    total = len(results)
    
    print("=" * 50)
    print(f"Integration test results: {passed}/{total} passed")
    
    if passed == total:
        print("✓ All tests passed! manim-mcp-server should work correctly.")
        return 0
    else:
        print("✗ Some tests failed. Check the output above.")
        return 1

if __name__ == "__main__":
    sys.exit(main())