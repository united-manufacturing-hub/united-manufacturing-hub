"""
This file provides helm linting and lint reporting
"""
import os
import subprocess
from pathlib import Path

from .git import Git
from .helper import Progressbar
from .log import Colors
from .log import Log
from .ilib import LibInterface


class LibHelmLint(LibInterface):
    lints: dict

    def __init__(self):
        """
        This class executes helm lint on changed files and reports its findings
        """
        """
        Initializes config parameters and gets changed files
        """
        self.chart_files = []

        # Check if current branch has upstream
        # If so, only include changed Charts
        # If not check all Charts
        self.chart_files = []
        if Git.has_upstream():
            changes = Git.get_committed_changes()
            for change in changes:
                if change.endswith("Chart.yaml"):
                    path = f"{Git.get_repository_root()}/{change}"
                    if not os.path.isfile(path):
                        Log.warn(f"Skipping non-existing file {path}")
                    else:
                        self.chart_files.append(path.replace("Chart.yaml",""))
        else:
            files = [str(path) for path in
                     list(Path(Git.get_repository_root()).rglob('Chart.yaml'))]
            for path in files:
                if not os.path.isfile(path):
                    Log.warn(f"Skipping non-existing file {path}")
                else:
                    self.chart_files.append(path.replace("Chart.yaml", ""))

        self.chart_files = list(dict.fromkeys(self.chart_files))

        self.lints = dict()

    def check(self):
        """
        Applies helm lint to all Chart.yaml files that should be checked
        and parses the output into an dict easier handling
        :return:
        """
        ly = len(self.chart_files)
        if ly == 0:
            Log.info("No helm charts to check")
            return

        Log.info(f"Checking {ly} chart files")

        pb = Progressbar(ly)
        for path in self.chart_files:
            self.lints[path] = dict()
            self.lints[path]["warn"] = []
            self.lints[path]["error"] = []
            try:
                output = subprocess.check_output(
                    ['helm', 'lint', path], stderr=subprocess.STDOUT, shell=True,
                    universal_newlines=True)
            except subprocess.CalledProcessError as exc:
                lines = exc.output.splitlines()
            else:
                lines = output.splitlines()
            for line in lines:
                line = str(line)
                if "[ERROR]" in line:
                    self.lints[path]["error"].append(line)
                elif "[WARNING]" in line:
                    self.lints[path]["warn"].append(line)
            pb.add_progress()
        pb.finish()

    def report(self) -> int:
        """
        Prints results of lint checking in an formatted way.
        :return: number of errors found
        """
        if len(self.chart_files) == 0:
            return 0
        errors = 0
        warnings = 0
        for path, lint in self.lints.items():
            Log.info(f"{path}")
            for lint_message in lint["error"]:
                Log.fail(f"\t{lint_message}")
                errors += 1

            for lint_message in lint["warn"]:
                Log.warn(f"\t{lint_message}")
                warnings += 1

        print()
        if errors > 0:
            failstr = f"|| Helm lint failed with {errors} errors and {Colors.WARNING}{warnings} warnings{Colors.ENDC}{Colors.FAIL} ||"
            fstrlen = len(failstr) - 14
            Log.fail('=' * fstrlen)
            Log.fail(failstr)
            Log.fail('=' * fstrlen)

        elif warnings > 0:
            warnstr = f"Helm lint succeeded with {warnings} warnings"
            Log.warn('=' * len(warnstr))
            Log.warn(warnstr)
            Log.warn('=' * len(warnstr))
        else:
            Log.ok("====================")
            Log.ok(f"Helm lint succeeded")
            Log.ok("====================")

        return errors

    def run(self):
        """
        Runs check & report
        :return: reports return value
        """
        self.check()
        return self.report()
