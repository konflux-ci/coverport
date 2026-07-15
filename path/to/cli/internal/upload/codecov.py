import os
import json
import requests

def upload_coverage(coverage_file, flags):
    try:
        with open(coverage_file, 'r') as f:
            coverage_data = json.load(f)
    except FileNotFoundError:
        print(f"Error: Coverage file '{coverage_file}' not found.")
        return

    try:
        response = requests.post(
            'https://codecov.io',
            data={'flags': flags},
            files={'coverage': open(coverage_file, 'rb')}
        )
        response.raise_for_status()
    except requests.exceptions.RequestException as e:
        print(f"Error uploading coverage: {e}")
    else:
        print("Coverage uploaded successfully.")

def main():
    coverage_file = 'coverage.out'
    flags = 'cli-tests'
    upload_coverage(coverage_file, flags)

if __name__ == '__main__':
    main()