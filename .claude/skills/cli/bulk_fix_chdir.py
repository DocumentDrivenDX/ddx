#!/usr/bin/env python3

import re
import os
import sys

def emit(message):
    sys.stdout.write(f"{message}\n")

def fix_test_file(filepath):
    """Fix os.Chdir patterns in a single test file."""
    
    with open(filepath, 'r') as f:
        content = f.read()
    
    original_content = content
    
    # Pattern 1: Replace the standard test pattern
    # origDir, _ := os.Getwd()
    # defer os.Chdir(origDir)
    # tempDir := t.TempDir()
    # os.Chdir(tempDir)
    
    pattern1 = r'(\s+)(?:\/\/ Save and restore working directory\s+)?origDir, _ := os\.Getwd\(\)\s+defer os\.Chdir\(origDir\)\s+(?:\/\/ Given: .*\s+)?tempDir := t\.TempDir\(\)\s+os\.Chdir\(tempDir\)'
    
    replacement1 = r'\1WithIsolatedDirectory(t, func(workingDir string) {'
    content = re.sub(pattern1, replacement1, content, flags=re.MULTILINE)
    
    # Pattern 2: Replace getFreshRootCmd() with GetCommandInDirectory(workingDir)
    # This should only be done within WithIsolatedDirectory blocks
    content = re.sub(r'getFreshRootCmd\(\)', r'GetCommandInDirectory(workingDir)', content)
    
    # Pattern 3: Replace file path references to use workingDir
    content = re.sub(r'assert\.FileExists\(t, "\.ddx\.yml"', r'assert.FileExists(t, filepath.Join(workingDir, ".ddx.yml")', content)
    content = re.sub(r'os\.WriteFile\("\.ddx\.yml"', r'os.WriteFile(filepath.Join(workingDir, ".ddx.yml")', content)
    content = re.sub(r'os\.ReadFile\("\.ddx\.yml"', r'os.ReadFile(filepath.Join(workingDir, ".ddx.yml")', content)
    
    if content != original_content:
        # Create backup
        with open(filepath + '.backup', 'w') as f:
            f.write(original_content)
        
        # Write fixed content
        with open(filepath, 'w') as f:
            f.write(content)
        
        return True
    return False

def main():
    """Fix all test files in the cmd directory."""
    
    cmd_dir = 'cmd'
    if not os.path.exists(cmd_dir):
        emit("cmd directory not found!")
        return 1
    
    fixed_count = 0
    
    for filename in os.listdir(cmd_dir):
        if filename.endswith('_test.go'):
            filepath = os.path.join(cmd_dir, filename)
            if fix_test_file(filepath):
                emit(f"Fixed {filename}")
                fixed_count += 1
            else:
                emit(f"No changes needed for {filename}")

    emit("")
    emit(f"Fixed {fixed_count} test files")
    return 0

if __name__ == '__main__':
    sys.exit(main())
