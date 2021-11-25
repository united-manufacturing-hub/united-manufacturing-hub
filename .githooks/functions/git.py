import subprocess


def memoize(function):
    memo = {}

    def wrapper(*args):
        if args in memo:
            return memo[args]
        else:
            rv = function(*args)
            memo[args] = rv
            return rv

    return wrapper


class Git:
    @staticmethod
    @memoize
    def get_current_branch() -> str:
        output = subprocess.check_output(['git', 'rev-parse', '--abbrev-ref', 'HEAD']).decode("UTF-8").strip()
        return output

    @staticmethod
    @memoize
    def has_upstream() -> bool:
        remote_branches = subprocess.check_output(['git', 'branch', '-r']).decode("UTF-8").strip()
        current_branch = Git.get_current_branch()
        return current_branch in remote_branches

    @staticmethod
    @memoize
    def get_committed_changes() -> [str]:
        if not Git.has_upstream():
            return []
        current_branch = Git.get_current_branch()
        changes_str = subprocess.check_output(
            ['git', 'log', '--name-only', '--pretty=format:', f"origin/{current_branch}..HEAD"]).decode("UTF-8").strip()
        return changes_str.splitlines()

    @staticmethod
    @memoize
    def get_repository_root() -> str:
        return subprocess.check_output(['git', 'rev-parse', '--show-toplevel']).decode("UTF-8").strip()
