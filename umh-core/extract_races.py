#!/usr/bin/env python3
"""
Script to extract data races from result.txt and save them to races.txt
"""

def extract_data_races():
    with open('result.txt', 'r') as f:
        content = f.read()
    
    races = []
    lines = content.split('\n')
    
    i = 0
    while i < len(lines):
        # Look for the start of a data race report
        if lines[i].strip() == '==================' and i + 1 < len(lines) and 'WARNING: DATA RACE' in lines[i + 1]:
            # Found the start of a data race
            race_start = i
            
            # Find the end of the data race (next ==================)
            j = i + 2
            while j < len(lines) and lines[j].strip() != '==================':
                j += 1
            
            if j < len(lines):
                race_end = j
                race_content = '\n'.join(lines[race_start:race_end + 1])
                races.append(race_content)
                i = j + 1
            else:
                break
        else:
            i += 1
    
    # Write all races to races.txt
    with open('races.txt', 'w') as f:
        for idx, race in enumerate(races, 1):
            f.write(f"DATA RACE #{idx}\n")
            f.write(race)
            f.write('\n\n')
    
    print(f"Extracted {len(races)} data races to races.txt")

if __name__ == "__main__":
    extract_data_races() 