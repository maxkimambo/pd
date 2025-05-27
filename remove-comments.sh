#!/bin/bash

# Exclude common directories like .git, node_modules, dist.
# Process all other files.
find . \( -name ".git" -o -name "node_modules" -o -name "dist" -o -name "bin" -o -name "vendor" \) -prune -o -type f -print0 | xargs -0 bash -c '
  for file do
    # Defensive check to ensure we are not processing files inside excluded directories
    # This is somewhat redundant given the find -prune, but adds an extra layer of safety.
    case "$file" in
      */.git/*|*/node_modules/*|*/dist/*|*/bin/*|*/vendor/*)
        continue
        ;;
    esac

    # Attempt to remove lines that consist only of a // comment (possibly with leading whitespace)
    sed -i".sedbak" -e "/^[[:space:]]*\/\/.*/d" "$file"
    # Check if sed created a backup (meaning it made a change) before trying to remove it
    if [ -f "${file}.sedbak" ]; then
        rm -f "${file}.sedbak"
    fi

    # Attempt to remove trailing // comments from other lines
    # s/pattern/replacement/g - g for global if multiple on one line (though // usually isn't)
    sed -i".sedbak" -e "s/\/\/[^"]*$/\/\//g; s/\/\/.*//g" "$file"
    # Check if sed created a backup
    if [ -f "${file}.sedbak" ]; then
        rm -f "${file}.sedbak"
    fi

    # Optional: remove lines that became empty after comment removal
    # sed -i".sedbak" -e "/^[[:space:]]*$/d" "$file"
    # if [ -f "${file}.sedbak" ]; then
    #     rm -f "${file}.sedbak"
    # fi

  done
' _ # The underscore is a placeholder for $0 for the inline script

echo "Comment removal process attempted."
echo "WARNING: Review files carefully. This script can break code if '//' appears in non-comment contexts."