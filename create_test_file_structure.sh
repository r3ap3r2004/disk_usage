#!/bin/bash
# Loop through directories 01 to 10
for d in {1..10}; do
  # Format the directory name with two digits
  dir_name=$(printf "test_directories/directory_%02d" "$d")

  # Create the directory only if it does not exist
  if [ ! -d "$dir_name" ]; then
    mkdir -p "$dir_name"
  fi

  # Loop through files 01 to 10 in each directory
  for f in {1..10}; do
    # Format the file name with two digits
    file_name=$(printf "file_%02d" "$f")
    file_path="$dir_name/$file_name"

    # Check if the file exists; if not, create it with the specified size.
    if [ ! -f "$file_path" ]; then
      # Calculate size in kilobytes (directory number * file number)
      size_kb=$((d * f))
      # Create the file using dd, reading from /dev/zero.
      dd if=/dev/zero of="$file_path" bs=1024 count="$size_kb" status=none
    fi
  done
done
