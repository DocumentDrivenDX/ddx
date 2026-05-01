#!/bin/bash

# Update test files to use CommandFactory

echo "Updating test files to use CommandFactory..."

# List of test files to update
test_files=(
    "cmd/apply_test.go"
    "cmd/config_test.go"
    "cmd/contract_test.go"
    "cmd/init_test.go"
    "cmd/list_test.go"
    "cmd/persona_contract_test.go"
    "cmd/persona_acceptance_test.go"
    "cmd/persona_integration_test.go"
    "cmd/performance_test.go"
    "cmd/security_test.go"
)

for file in "${test_files[@]}"; do
    if [ -f "$file" ]; then
        echo "Processing $file..."

        # Check if file already uses CommandFactory
        if grep -q "NewCommandFactory()" "$file"; then
            echo "  Already uses CommandFactory, skipping..."
            continue
        fi

        # Check if file has helper function to create rootCmd
        if grep -q "func.*getRootCommand\|func.*newRootCommand" "$file"; then
            echo "  Has helper function, needs manual review..."
            continue
        fi

        # For simple cases where rootCmd is created inline
        if grep -q "rootCmd := &cobra.Command{" "$file"; then
            echo "  Found inline rootCmd creation, needs transformation..."
            # We'll handle these manually to ensure correctness
        fi
    fi
done

echo "Review complete. Manual updates needed for proper isolation."