import argparse
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


SPEC = importlib.util.spec_from_file_location('preview_service', Path(__file__).with_name('workspace-preview-service.py'))
service = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(service)


class FixtureBoundaryTests(unittest.TestCase):
    def arguments(self, directory):
        return argparse.Namespace(output=Path(directory)/'new-case', source=Path(directory)/'source',
                                  image='sha256:'+'a'*64, expected_source='b'*64, expected_helper='c'*64)

    def test_invalid_identity_fails_before_docker(self):
        with tempfile.TemporaryDirectory() as directory:
            args = self.arguments(directory)
            args.image = 'runcove-server:latest'
            with patch.object(service, 'load_owner') as owner:
                with self.assertRaises(ValueError):
                    service.run(args)
                owner.assert_not_called()
            self.assertFalse(args.output.exists())

    def test_occupied_evidence_is_preserved(self):
        with tempfile.TemporaryDirectory() as directory:
            args = self.arguments(directory)
            args.output.mkdir()
            evidence = args.output/'sentinel.json'
            evidence.write_text('preserve')
            with patch.object(service, 'load_owner') as owner:
                with self.assertRaises(FileExistsError):
                    service.run(args)
                owner.assert_not_called()
            self.assertEqual(evidence.read_text(), 'preserve')

    def test_startup_failure_cleans_registered_resources_and_keeps_failure(self):
        class Owner:
            instance = None
            def __init__(self, command, identity):
                Owner.instance = self
                self.containers, self.volumes, self.cleaned = [], [], False
            def volume(self, name): self.volumes.append(name)
            def container(self, name, arguments): self.containers.append(name)
            def cleanup(self): self.cleaned = True; return []

        def docker(*args, **kwargs):
            if args[:2] == ('image', 'inspect'):
                return json.dumps([{'Id':'sha256:'+'a'*64,'Architecture':'amd64','Os':'linux',
                                    'Config':{'Labels':{'io.runcove.target':'server','io.runcove.source-sha256':'b'*64}}}])
            if args[:1] == ('start',): raise RuntimeError('simulated startup failure')
            return ''

        with tempfile.TemporaryDirectory() as directory:
            args = self.arguments(directory)
            with patch.object(service, 'load_owner', return_value=Owner), patch.object(service, 'docker_command', return_value=docker):
                with self.assertRaisesRegex(RuntimeError, 'simulated startup'):
                    service.run(args)
            self.assertTrue(Owner.instance.cleaned)
            self.assertEqual(len(Owner.instance.volumes), 1)
            self.assertEqual(len(Owner.instance.containers), 1)
            evidence = json.loads((args.output/'fixture.json').read_text())
            self.assertFalse(evidence['ready'])
            self.assertTrue(evidence['cleanupPassed'])


if __name__ == '__main__':
    unittest.main()
