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
    def get_current_branch():
        output = subprocess.check_output(['git', 'rev-parse', '--abbrev-ref', 'HEAD']).decode("UTF-8").strip()
        return output

    @staticmethod
    @memoize
    def has_upstream():
        remote_branches = subprocess.check_output(['git', 'branch', '-r']).decode("UTF-8").strip()
        current_branch = Git.get_current_branch()
        return current_branch in remote_branches

    @staticmethod
    @memoize
    def get_repository_root():
        return subprocess.check_output(['git', 'rev-parse', '--show-toplevel']).decode("UTF-8").strip()

