"""Execute the generated selector in a real PTY, without installer host writes."""
import errno
import os
from pathlib import Path
import pty
import select
import signal
import subprocess
import time
import unittest

SOURCE = (Path(__file__).resolve().parents[3] / 'install.sh').read_text()
HELPER = SOURCE.split("cat <<'BAFT_MENU_READER'\n", 1)[1].split('\nBAFT_MENU_READER', 1)[0]
COMMAND = HELPER + '\nanswer=; baft_menu_read answer "1:status" "2:network" "12:advanced" "0:exit"; rc=$?; printf "\\nRESULT:%s:%s\\n" "$rc" "$answer"'


class MenuNavigation(unittest.TestCase):
    def interact(self, keys, term='xterm', expected_prompt=b'Choose ['):
        pid, fd = pty.fork()
        if pid == 0:
            os.environ['TERM'] = term
            os.execvp('bash', ['bash', '-c', COMMAND])
        output = b''
        deadline = time.monotonic() + 5
        sent = False
        try:
            while time.monotonic() < deadline:
                if select.select([fd], [], [], .1)[0]:
                    try:
                        part = os.read(fd, 8192)
                    except OSError as error:
                        if error.errno == errno.EIO:
                            break
                        raise
                    if not part:
                        break
                    output += part
                    if not sent and expected_prompt in output:
                        os.write(fd, keys)
                        sent = True
            else:
                self.fail('selector timed out: ' + repr(output))
        finally:
            os.close(fd)
            try:
                os.kill(pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            os.waitpid(pid, 0)
        self.assertTrue(sent, repr(output))
        return output

    def test_tab_enter(self):
        self.assertIn(b'RESULT:0:2', self.interact(b'\t\n'))

    def test_arrow_navigation(self):
        for keys, expected in [(b'\x1b[B\n', b'2'), (b'\x1b[A\n', b'0'),
                               (b'\x1bOB\n', b'2'), (b'\x1bOA\n', b'0'),
                               (b'\x1b[B' * 4 + b'\n', b'1'),
                               (b'\x1b[A' * 4 + b'\n', b'1'),
                               (b'12\x1b[B\n', b'2')]:
            with self.subTest(keys=keys):
                output = self.interact(keys)
                self.assertIn(b'RESULT:0:' + expected, output)
                self.assertIn(b'Up/Down', output)

    def test_unused_cursor_sequence_never_dispatches(self):
        self.assertIn(b'RESULT:0:1', self.interact(b'\x1b[D\n'))

    def test_shift_tab_wrap(self):
        self.assertIn(b'RESULT:0:0', self.interact(b'\x1b[Z\n'))

    def test_tab_wrap(self):
        self.assertIn(b'RESULT:0:1', self.interact(b'\t\t\t\t\n'))

    def test_numeric_multi_digit(self):
        self.assertIn(b'RESULT:0:12', self.interact(b'12\n'))

    def test_numeric_overrides_focus(self):
        self.assertIn(b'RESULT:0:12', self.interact(b'\t12\n'))

    def test_backspace(self):
        self.assertIn(b'RESULT:0:12', self.interact(b'13\x7f2\n'))

    def test_invalid_choice_never_dispatches(self):
        output = self.interact(b'99\n2\n')
        self.assertIn(b'Invalid choice', output)
        self.assertIn(b'RESULT:0:2', output)

    def test_dumb_has_no_escape_output(self):
        output = self.interact(b'12\n', term='dumb', expected_prompt=b'Choose:')
        self.assertIn(b'RESULT:0:12', output)
        self.assertNotIn(b'\x1b', output)

    def test_piped_number_and_eof(self):
        for data, result in [('12\n', 'RESULT:0:12'), ('', 'RESULT:1:')]:
            completed = subprocess.run(['bash', '-c', COMMAND], input=data, text=True,
                                       stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=3)
            self.assertIn(result, completed.stdout)
            self.assertNotIn('\x1b', completed.stderr)


if __name__ == '__main__':
    unittest.main()
