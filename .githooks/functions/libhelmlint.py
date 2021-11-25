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
        self.chart_files = [str(path).replace("Chart.yaml", "") for path in
                            list(Path(Git.get_repository_root()).rglob('Chart.yaml'))]
        self.lints = dict()

    def check(self):
        ly = len(self.chart_files)
        pb = Progressbar(ly)
        for path in self.chart_files:
            self.lints[path] = dict()
            self.lints[path]["warn"] = []
            self.lints[path]["error"] = []
            lines = []
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
        errors = 0
        warnings = 0
        for path, lint in self.lints.items():
            Log.info(f"{path}")
            for lint_message in lint["error"]:
                Log.fail(f"\t\t{lint_message}")
                errors += 1

            for lint_message in lint["warn"]:
                Log.warn(f"\t\t{lint_message}")
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
        self.check()
        return self.report()
