#!/usr/bin/env python3

# Copyright 2025 UMH Systems GmbH
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import re
import sys
from collections import defaultdict, Counter
import argparse
from datetime import datetime

class StraceAnalyzer:
    def __init__(self, log_file):
        self.log_file = log_file
        self.syscalls = []
        self.syscall_counts = Counter()
        self.syscall_times = defaultdict(list)
        self.errors = []
        self.file_operations = []
        self.file_timings = defaultdict(list)
        
    def parse_log(self):
        """Parse the strace log file and extract system calls"""
        pending_syscalls = {}  # Track unfinished syscalls by PID
        
        with open(self.log_file, 'r') as f:
            for line_num, line in enumerate(f, 1):
                line = line.strip()
                if not line or line.startswith('strace:'):
                    continue
                
                # Parse new format: PID TIMESTAMP SYSCALL
                # Example: "6024  09:32:23.204189 futex(0xc007400958, FUTEX_WAIT_PRIVATE, 0, NULL <unfinished ...>"
                match = re.match(r'^(\d+)\s+([\d:\.]+)\s+(.+)$', line)
                if not match:
                    continue
                    
                pid = match.group(1)
                timestamp = match.group(2)
                syscall_part = match.group(3)
                
                # Handle unfinished syscalls
                if '<unfinished ...>' in syscall_part:
                    # Extract syscall name from unfinished call
                    unfinished_match = re.match(r'([^(]+)\(([^<]*)<unfinished', syscall_part)
                    if unfinished_match:
                        syscall_name = unfinished_match.group(1)
                        partial_args = unfinished_match.group(2)
                        pending_syscalls[pid] = {
                            'name': syscall_name,
                            'partial_args': partial_args,
                            'start_line': line_num,
                            'timestamp': timestamp
                        }
                    continue
                
                # Handle resumed syscalls
                if 'resumed>' in syscall_part:
                    # Example: "<... clock_gettime resumed>{tv_sec=1718638, tv_nsec=405567251}) = 0 <0.000194>"
                    resumed_match = re.match(r'<\.\.\. ([^>]+) resumed>([^=]*)=\s*(.+)', syscall_part)
                    if resumed_match and pid in pending_syscalls:
                        syscall_name = resumed_match.group(1)
                        remaining_args = resumed_match.group(2)
                        result = resumed_match.group(3)
                        
                        # Get the pending syscall info
                        pending = pending_syscalls[pid]
                        full_args = pending['partial_args'] + remaining_args
                        
                        # Extract timing from result
                        execution_time = None
                        timing_match = re.search(r'<([\d.]+)>$', result)
                        if timing_match:
                            execution_time = float(timing_match.group(1))
                            result = re.sub(r'\s*<[\d.]+>$', '', result)
                        
                        # Handle clock_gettime timing extraction
                        timing = None
                        if syscall_name == 'clock_gettime' and 'tv_sec=' in full_args:
                            sec_match = re.search(r'tv_sec=(\d+)', full_args)
                            nsec_match = re.search(r'tv_nsec=(\d+)', full_args)
                            if sec_match and nsec_match:
                                timing = int(sec_match.group(1)) + int(nsec_match.group(1)) / 1e9
                        
                        syscall_info = {
                            'line': line_num,
                            'name': syscall_name,
                            'args': full_args.strip(),
                            'result': result.strip(),
                            'timing': timing,
                            'execution_time': execution_time,
                            'pid': pid,
                            'timestamp': timestamp
                        }
                        
                        self.process_syscall(syscall_info)
                        del pending_syscalls[pid]
                        continue
                
                # Handle complete syscalls in one line
                # Example: "3056  09:32:23.204228 clock_gettime(CLOCK_MONOTONIC, {...}) = 0 <0.000001>"
                complete_match = re.match(r'([^(]+)\(([^)]*)\)\s*=\s*(.+)', syscall_part)
                if complete_match:
                    syscall_name = complete_match.group(1)
                    args = complete_match.group(2)
                    result = complete_match.group(3)
                    
                    # Extract timing from result
                    execution_time = None
                    timing_match = re.search(r'<([\d.]+)>$', result)
                    if timing_match:
                        execution_time = float(timing_match.group(1))
                        result = re.sub(r'\s*<[\d.]+>$', '', result)
                    
                    # Extract timing information if present in args (clock_gettime)
                    timing = None
                    if 'tv_sec=' in args and 'tv_nsec=' in args:
                        sec_match = re.search(r'tv_sec=(\d+)', args)
                        nsec_match = re.search(r'tv_nsec=(\d+)', args)
                        if sec_match and nsec_match:
                            timing = int(sec_match.group(1)) + int(nsec_match.group(1)) / 1e9
                    
                    syscall_info = {
                        'line': line_num,
                        'name': syscall_name,
                        'args': args,
                        'result': result,
                        'timing': timing,
                        'execution_time': execution_time,
                        'pid': pid,
                        'timestamp': timestamp
                    }
                    
                    self.process_syscall(syscall_info)
    
    def process_syscall(self, syscall_info):
        """Process a parsed syscall and update counters/collections"""
        self.syscalls.append(syscall_info)
        self.syscall_counts[syscall_info['name']] += 1
        
        if syscall_info['timing']:
            self.syscall_times[syscall_info['name']].append(syscall_info['timing'])
        
        # Track file operations with timing
        if syscall_info['name'] in ['open', 'openat', 'read', 'write', 'close', 'newfstatat', 'access', 'fcntl']:
            file_op = {
                'syscall': syscall_info['name'],
                'args': syscall_info['args'],
                'result': syscall_info['result'],
                'execution_time': syscall_info['execution_time'],
                'line': syscall_info['line'],
                'pid': syscall_info.get('pid', 'unknown'),
                'timestamp': syscall_info.get('timestamp', 'unknown')
            }
            self.file_operations.append(file_op)
            
            if syscall_info['execution_time']:
                self.file_timings[syscall_info['name']].append(syscall_info['execution_time'])
        
        # Track errors
        if '= -1' in syscall_info['result']:
            self.errors.append(syscall_info)
    
    def get_summary(self):
        """Generate a summary of the strace analysis"""
        total_syscalls = len(self.syscalls)
        unique_syscalls = len(self.syscall_counts)
        total_errors = len(self.errors)
        
        summary = f"""
STRACE ANALYSIS SUMMARY
=======================
Total system calls: {total_syscalls}
Unique system calls: {unique_syscalls}
Total errors: {total_errors}
Error rate: {(total_errors/total_syscalls)*100:.2f}%

TOP 10 MOST FREQUENT SYSTEM CALLS:
"""
        
        for syscall, count in self.syscall_counts.most_common(10):
            percentage = (count / total_syscalls) * 100
            summary += f"  {syscall:<20}: {count:>8} ({percentage:>5.1f}%)\n"
        
        return summary
    
    def get_errors(self):
        """Get detailed error information"""
        if not self.errors:
            return "No errors found in the trace."
        
        error_summary = f"\nERRORS FOUND ({len(self.errors)} total):\n"
        error_summary += "=" * 40 + "\n"
        
        error_counts = Counter()
        for error in self.errors:
            error_key = f"{error['name']} -> {error['result']}"
            error_counts[error_key] += 1
        
        for error_type, count in error_counts.most_common():
            error_summary += f"{error_type}: {count} occurrences\n"
        
        return error_summary
    
    def get_timing_analysis(self):
        """Analyze timing patterns for clock-related syscalls"""
        timing_summary = "\nDEEP TIMING ANALYSIS:\n"
        timing_summary += "=" * 25 + "\n"
        
        # Analyze clock_gettime patterns
        clock_calls = [s for s in self.syscalls if s['name'] == 'clock_gettime']
        if clock_calls:
            monotonic_calls = [s for s in clock_calls if 'CLOCK_MONOTONIC' in s['args']]
            realtime_calls = [s for s in clock_calls if 'CLOCK_REALTIME' in s['args']]
            
            timing_summary += f"Clock Analysis ({len(clock_calls)} total calls):\n"
            timing_summary += f"  MONOTONIC calls: {len(monotonic_calls)} ({len(monotonic_calls)/len(clock_calls)*100:.1f}%)\n"
            timing_summary += f"  REALTIME calls: {len(realtime_calls)} ({len(realtime_calls)/len(clock_calls)*100:.1f}%)\n"
            
            # Calculate call frequency
            if len(clock_calls) > 1:
                # Extract timestamps and calculate intervals
                monotonic_times = []
                for call in monotonic_calls[:1000]:  # Sample first 1000 for performance
                    sec_match = re.search(r'tv_sec=(\d+)', call['args'])
                    nsec_match = re.search(r'tv_nsec=(\d+)', call['args'])
                    if sec_match and nsec_match:
                        timestamp = int(sec_match.group(1)) + int(nsec_match.group(1)) / 1e9
                        monotonic_times.append(timestamp)
                
                if len(monotonic_times) > 1:
                    intervals = [monotonic_times[i+1] - monotonic_times[i] 
                               for i in range(len(monotonic_times)-1) if monotonic_times[i+1] > monotonic_times[i]]
                    if intervals:
                        avg_interval = sum(intervals) / len(intervals)
                        min_interval = min(intervals)
                        max_interval = max(intervals)
                        frequency = 1.0 / avg_interval if avg_interval > 0 else 0
                        
                        timing_summary += f"  Average interval: {avg_interval*1000:.3f}ms\n"
                        timing_summary += f"  Min interval: {min_interval*1000:.3f}ms\n"
                        timing_summary += f"  Max interval: {max_interval*1000:.3f}ms\n"
                        timing_summary += f"  Estimated frequency: {frequency:.1f} Hz\n"
        
        # Analyze rt_sigreturn patterns
        sigreturn_calls = [s for s in self.syscalls if s['name'] == 'rt_sigreturn']
        if sigreturn_calls:
            timing_summary += f"\nSignal Analysis ({len(sigreturn_calls)} rt_sigreturn calls):\n"
            timing_summary += f"  Signal mask patterns:\n"
            
            mask_patterns = {}
            for call in sigreturn_calls:
                mask = call['args']
                mask_patterns[mask] = mask_patterns.get(mask, 0) + 1
            
            for pattern, count in mask_patterns.items():
                timing_summary += f"    {pattern}: {count} times\n"
        
        return timing_summary
    
    def get_behavioral_analysis(self):
        """Analyze behavioral patterns and potential reasons for high syscall frequency"""
        analysis = "\nBEHAVIORAL ANALYSIS:\n"
        analysis += "=" * 22 + "\n"
        
        # Look for patterns that suggest specific behaviors
        futex_calls = [s for s in self.syscalls if s['name'] == 'futex']
        nanosleep_calls = [s for s in self.syscalls if s['name'] == 'nanosleep']
        epoll_calls = [s for s in self.syscalls if s['name'] == 'epoll_pwait']
        
        # Analyze timing call patterns
        clock_calls = [s for s in self.syscalls if s['name'] == 'clock_gettime']
        if clock_calls:
            analysis += f"High-frequency timing pattern detected:\n"
            analysis += f"  {len(clock_calls)} clock_gettime calls suggest:\n"
            
            # Check for performance monitoring
            if len(clock_calls) > 50000:
                analysis += f"  • PERFORMANCE MONITORING: Very high frequency timing calls\n"
                analysis += f"  • Likely measuring execution time, latency, or throughput\n"
            
            # Check for timeout/polling behavior
            if nanosleep_calls and len(nanosleep_calls) > 100:
                analysis += f"  • POLLING BEHAVIOR: {len(nanosleep_calls)} nanosleep calls detected\n"
                
                # Sample some nanosleep durations
                sleep_durations = []
                for call in nanosleep_calls[:50]:
                    nsec_match = re.search(r'tv_nsec=(\d+)', call['args'])
                    if nsec_match:
                        duration_ns = int(nsec_match.group(1))
                        sleep_durations.append(duration_ns)
                
                if sleep_durations:
                    avg_sleep = sum(sleep_durations) / len(sleep_durations) / 1000000  # Convert to ms
                    analysis += f"    - Average sleep duration: {avg_sleep:.2f}ms\n"
                    analysis += f"    - Suggests active polling with short intervals\n"
        
        # Analyze signal patterns
        sigreturn_calls = [s for s in self.syscalls if s['name'] == 'rt_sigreturn']
        if sigreturn_calls and len(sigreturn_calls) > 10000:
            analysis += f"\nHigh-frequency signal handling detected:\n"
            analysis += f"  {len(sigreturn_calls)} rt_sigreturn calls suggest:\n"
            analysis += f"  • TIMER-BASED EXECUTION: Likely using signals for scheduling\n"
            analysis += f"  • Could indicate Go runtime goroutine scheduling\n"
            analysis += f"  • Or system timer interrupts for precise timing\n"
        
        # Analyze synchronization patterns
        if futex_calls:
            futex_wait = len([s for s in futex_calls if 'FUTEX_WAIT' in s['args']])
            futex_wake = len([s for s in futex_calls if 'FUTEX_WAKE' in s['args']])
            analysis += f"\nSynchronization patterns:\n"
            analysis += f"  • FUTEX_WAIT calls: {futex_wait}\n"
            analysis += f"  • FUTEX_WAKE calls: {futex_wake}\n"
            if futex_wait > 1000:
                analysis += f"  • Heavy thread synchronization/blocking detected\n"
                analysis += f"  • Suggests multi-threaded application with coordination\n"
        
        # Analyze I/O patterns
        if epoll_calls:
            analysis += f"\nI/O Event handling:\n"
            analysis += f"  • {len(epoll_calls)} epoll_pwait calls detected\n"
            analysis += f"  • Indicates event-driven I/O (likely network operations)\n"
        
        # Look for service/container patterns from file paths
        service_files = [s for s in self.syscalls if s['name'] in ['newfstatat', 'openat'] and '/run/service/' in s['args']]
        if service_files:
            analysis += f"\nService management activity:\n"
            analysis += f"  • {len(service_files)} service-related file operations\n"
            analysis += f"  • Suggests system service supervision/monitoring\n"
        
        # Overall assessment
        analysis += f"\nLIKELY SCENARIO:\n"
        total_calls = len(self.syscalls)
        timing_percentage = len(clock_calls) / total_calls * 100 if clock_calls else 0
        signal_percentage = len(sigreturn_calls) / total_calls * 100 if sigreturn_calls else 0
        
        if timing_percentage > 40 and signal_percentage > 20:
            analysis += f"  This appears to be a Go-based microservice with:\n"
            analysis += f"  • High-precision timing requirements ({timing_percentage:.1f}% timing calls)\n"
            analysis += f"  • Timer-driven goroutine scheduling ({signal_percentage:.1f}% signals)\n"
            analysis += f"  • Active monitoring/metrics collection\n"
            analysis += f"  • Service supervision (s6/daemontools pattern)\n"
            analysis += f"\n  Recommendations:\n"
            analysis += f"  • Consider reducing metrics collection frequency if performance is a concern\n"
            analysis += f"  • Monitor for busy-wait loops in polling logic\n"
            analysis += f"  • Check if timer precision requirements are actually needed\n"
        
        return analysis
    
    def get_file_timing_analysis(self):
        """Analyze file I/O operation timing and latencies"""
        if not self.file_operations:
            return "No file operations found."
        
        timing_analysis = f"\nFILE I/O TIMING ANALYSIS:\n"
        timing_analysis += "=" * 30 + "\n"
        
        # Count operations with timing data
        timed_ops = [op for op in self.file_operations if op['execution_time'] is not None]
        timing_analysis += f"File operations with timing data: {len(timed_ops)} / {len(self.file_operations)}\n\n"
        
        if not timed_ops:
            timing_analysis += "No timing data available (strace may not have been run with -T option)\n"
            timing_analysis += "To get timing data, run: strace -T -o strace.log <command>\n\n"
            
            # Still provide some useful analysis even without timing
            timing_analysis += "Available analysis without timing data:\n"
            timing_analysis += "-" * 40 + "\n"
            
            # Count by syscall type
            syscall_counts = Counter()
            for op in self.file_operations:
                syscall_counts[op['syscall']] += 1
            
            timing_analysis += "File operation frequency:\n"
            for syscall, count in syscall_counts.most_common():
                timing_analysis += f"  {syscall:>12}: {count:>5} calls\n"
            
            # Analyze file paths accessed
            timing_analysis += f"\nFile access patterns:\n"
            path_counts = defaultdict(int)
            for op in self.file_operations:
                path_match = re.search(r'"([^"]*)"', op['args'])
                if path_match:
                    path = path_match.group(1)
                    if '/run/service/' in path:
                        key = "/run/service/* (service mgmt)"
                    elif '/data/logs/' in path:
                        key = "/data/logs/* (log files)"
                    elif path.startswith('/proc/'):
                        key = "/proc/* (process info)"
                    else:
                        key = path[:50] + "..." if len(path) > 50 else path
                    path_counts[key] += 1
            
            for path, count in sorted(path_counts.items(), key=lambda x: x[1], reverse=True)[:10]:
                timing_analysis += f"  {count:>3}x {path}\n"
            
            return timing_analysis
        
        # Analyze by syscall type
        timing_analysis += "Latency breakdown by syscall type:\n"
        timing_analysis += "-" * 40 + "\n"
        
        for syscall_type in sorted(self.file_timings.keys()):
            times = self.file_timings[syscall_type]
            if times:
                count = len(times)
                avg_time = sum(times) / count
                min_time = min(times)
                max_time = max(times)
                p95_time = sorted(times)[int(count * 0.95)] if count > 20 else max_time
                
                timing_analysis += f"{syscall_type:>12}: {count:>5} calls\n"
                timing_analysis += f"              Avg: {avg_time*1000:>7.3f}ms"
                timing_analysis += f"  Min: {min_time*1000:>7.3f}ms"
                timing_analysis += f"  Max: {max_time*1000:>7.3f}ms"
                timing_analysis += f"  P95: {p95_time*1000:>7.3f}ms\n"
        
        # Identify slow operations
        slow_ops = [op for op in timed_ops if op['execution_time'] > 0.01]  # > 10ms
        if slow_ops:
            timing_analysis += f"\nSlow file operations (>10ms):\n"
            timing_analysis += "-" * 40 + "\n"
            
            # Sort by execution time, show top 10
            slow_ops.sort(key=lambda x: x['execution_time'], reverse=True)
            for op in slow_ops[:10]:
                timing_analysis += f"Line {op['line']:>5}: {op['syscall']} "
                timing_analysis += f"({op['execution_time']*1000:.1f}ms)\n"
                
                # Extract file path from args
                path_match = re.search(r'"([^"]*)"', op['args'])
                if path_match:
                    path = path_match.group(1)
                    if len(path) > 60:
                        path = "..." + path[-57:]
                    timing_analysis += f"           {path}\n"
        
        # File path timing analysis
        timing_analysis += f"\nFile path timing patterns:\n"
        timing_analysis += "-" * 40 + "\n"
        
        path_timings = defaultdict(list)
        for op in timed_ops:
            # Extract file path from args
            path_match = re.search(r'"([^"]*)"', op['args'])
            if path_match:
                path = path_match.group(1)
                # Group similar paths
                if '/run/service/' in path:
                    key = "/run/service/* (service files)"
                elif '/data/logs/' in path:
                    key = "/data/logs/* (log files)"  
                elif path.startswith('/proc/'):
                    key = "/proc/* (process info)"
                elif path.startswith('/sys/'):
                    key = "/sys/* (system info)"
                else:
                    key = path if len(path) < 50 else "..." + path[-47:]
                
                path_timings[key].append(op['execution_time'])
        
        # Show top paths by average latency
        path_stats = []
        for path, times in path_timings.items():
            if len(times) >= 5:  # Only paths with enough samples
                avg_time = sum(times) / len(times)
                path_stats.append((path, len(times), avg_time, max(times)))
        
        path_stats.sort(key=lambda x: x[2], reverse=True)  # Sort by average time
        
        for path, count, avg_time, max_time in path_stats[:10]:
            timing_analysis += f"{count:>4} ops: {avg_time*1000:>6.2f}ms avg, {max_time*1000:>6.2f}ms max\n"
            timing_analysis += f"         {path}\n"
        
        return timing_analysis
    
    def get_file_operations(self):
        """Extract file operations from the trace"""
        file_ops = []
        file_syscalls = ['open', 'openat', 'newfstatat', 'read', 'write', 'close', 'access']
        
        for syscall in self.syscalls:
            if syscall['name'] in file_syscalls:
                file_ops.append(syscall)
        
        if not file_ops:
            return "No file operations found."
        
        file_summary = f"\nFILE OPERATIONS ({len(file_ops)} total):\n"
        file_summary += "=" * 30 + "\n"
        
        # Extract file paths
        file_paths = set()
        for op in file_ops:
            # Extract quoted paths from arguments
            path_matches = re.findall(r'"([^"]*)"', op['args'])
            for path in path_matches:
                if path and not path.startswith('/proc'):  # Filter out proc paths
                    file_paths.add(path)
        
        if file_paths:
            file_summary += "Files accessed:\n"
            for path in sorted(file_paths):
                file_summary += f"  {path}\n"
        
        return file_summary

def main():
    parser = argparse.ArgumentParser(
        description='Analyze strace log files for performance insights',
        epilog='''
Examples:
  %(prog)s trace.log                    # Full analysis (summary + errors + behavior)
  %(prog)s trace.log --summary          # Just summary statistics
  %(prog)s trace.log --timing           # Clock and timing analysis  
  %(prog)s trace.log --behavior         # Behavioral analysis and recommendations
  %(prog)s trace.log --file-times       # File I/O latency analysis
  
To capture strace data with timing:
  strace -T -o trace.log <command>      # With execution times
  strace -tt -T -o trace.log <command>  # With absolute timestamps + times
        ''',
        formatter_class=argparse.RawDescriptionHelpFormatter
    )
    parser.add_argument('logfile', help='Path to the strace log file')
    parser.add_argument('--summary', action='store_true', help='Show summary only')
    parser.add_argument('--errors', action='store_true', help='Show errors only')
    parser.add_argument('--timing', action='store_true', help='Show timing analysis only')
    parser.add_argument('--files', action='store_true', help='Show file operations only')
    parser.add_argument('--behavior', action='store_true', help='Show behavioral analysis only')
    parser.add_argument('--file-times', action='store_true', help='Show file I/O timing analysis only')
    
    args = parser.parse_args()
    
    try:
        analyzer = StraceAnalyzer(args.logfile)
        analyzer.parse_log()
        
        # Check if any specific flag is set (argparse converts dashes to underscores)
        file_times_flag = getattr(args, 'file_times', False)
        specific_flags = [args.errors, args.timing, args.files, args.behavior, file_times_flag]
        
        if args.summary or not any(specific_flags):
            print(analyzer.get_summary())
        
        if args.errors or not any([args.summary, args.timing, args.files, args.behavior, file_times_flag]):
            print(analyzer.get_errors())
        
        if args.timing:
            print(analyzer.get_timing_analysis())
        
        if args.files:
            print(analyzer.get_file_operations())
        
        if file_times_flag:
            print(analyzer.get_file_timing_analysis())
        
        if args.behavior or not any([args.summary, args.errors, args.timing, args.files, file_times_flag]):
            print(analyzer.get_behavioral_analysis())
            
    except FileNotFoundError:
        print(f"Error: File '{args.logfile}' not found.")
        sys.exit(1)
    except Exception as e:
        print(f"Error analyzing strace log: {e}")
        sys.exit(1)

if __name__ == '__main__':
    main()