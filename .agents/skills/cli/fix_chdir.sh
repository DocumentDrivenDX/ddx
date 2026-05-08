#!/bin/bash

# Script to systematically replace os.Chdir patterns in test files

for file in cmd/*_test.go; do
    if [[ -f "$file" ]]; then
        echo "Processing $file..."
        
        # Create backup
        cp "$file" "$file.backup"
        
        # Pattern 1: Replace the common defer os.Chdir(origDir) pattern
        sed -i 's/defer os\.Chdir(origDir)/\/\/ Removed defer os.Chdir(origDir) - using CommandFactory injection/' "$file"
        
        # Pattern 2: Replace os.Chdir(tempDir) with comment
        sed -i 's/os\.Chdir(tempDir)/\/\/ Removed os.Chdir(tempDir) - using CommandFactory injection/' "$file"
        
        # Pattern 3: Replace origDir, _ := os.Getwd() lines
        sed -i 's/origDir, _ := os\.Getwd()/\/\/ Removed origDir, _ := os.Getwd() - using CommandFactory injection/' "$file"
    fi
done

echo "Done! Backups created as *.backup files"
