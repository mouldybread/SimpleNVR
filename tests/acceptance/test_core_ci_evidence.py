"""Synthetic assessor controls, not execution evidence for the Go contracts."""
import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

spec=importlib.util.spec_from_file_location('core_evidence',Path(__file__).with_name('core_ci_evidence.py'))
m=importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
HEAD='a'*40
PACKAGE='example.test/nvr'


class AssessmentTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root=Path(self.tmp.name)
        self.meta={'schema':1,'head':HEAD,'root_package':PACKAGE,'command':m.COMMAND,
                   'test_exit':0,'tee_exit':0,'runtime_qualified':False,'cleanup_authorized':False}
        self.events=[{'Action':'start','Package':PACKAGE}]
        for name in list(m.REQUIRED)+[f'{m.FUZZ}/seed#{i}' for i in range(10)]:
            for action in ('run','pass'):
                self.events.append({'Action':action,'Package':PACKAGE,'Test':name})
        self.events.append({'Action':'pass','Package':PACKAGE})

    def write(self):
        (self.root/'run.json').write_text(json.dumps(self.meta))
        (self.root/'go-test.jsonl').write_text(''.join(json.dumps(e)+'\n' for e in self.events))

    def assess(self):
        self.write()
        return m.assess(self.root,HEAD)

    def test_positive_requires_all_tests_and_seeds(self):
        result=self.assess()
        self.assertEqual(result['required_top_level_tests'],7)
        self.assertEqual(result['required_fuzz_seeds'],10)
        self.assertFalse(result['runtime_qualified'] or result['cleanup_authorized'])

    def test_missing_or_skipped_required_test_or_seed(self):
        original=copy.deepcopy(self.events)
        for name in (m.REQUIRED[0],f'{m.FUZZ}/seed#9'):
            for mode in ('missing','skip','running'):
                with self.subTest(name=name,mode=mode):
                    self.events=copy.deepcopy(original)
                    if mode=='missing':
                        self.events=[e for e in self.events if e.get('Test')!=name]
                    else:
                        for e in self.events:
                            if e.get('Test')==name and e['Action']=='pass':
                                e['Action']='skip' if mode=='skip' else 'output'
                    with self.assertRaises(ValueError): self.assess()

    def test_command_identity_exit_and_authority_faults(self):
        faults=[('head','b'*40),('command',['go','test','./...']),('test_exit',1),
                ('tee_exit',1),('test_exit',False),('runtime_qualified',True),
                ('cleanup_authorized',True),('root_package','wrong/package')]
        original=copy.deepcopy(self.meta)
        for key,value in faults:
            with self.subTest(key=key,value=value):
                self.meta=copy.deepcopy(original);self.meta[key]=value
                with self.assertRaises(ValueError): self.assess()

    def test_failures_anywhere_refuse(self):
        for action in ('fail','build-fail'):
            self.events.insert(-1,{'Package':PACKAGE,'Action':action})
            with self.assertRaises(ValueError): self.assess()
            self.events.pop(-2)

    def test_duplicate_or_missing_lifecycle_refuses(self):
        for mutation in ('duplicate','no_run','no_package_pass'):
            with self.subTest(mutation=mutation):
                original=copy.deepcopy(self.events)
                if mutation=='duplicate': self.events.insert(2,copy.deepcopy(self.events[1]))
                elif mutation=='no_run': self.events.pop(1)
                else: self.events.pop()
                with self.assertRaises(ValueError): self.assess()
                self.events=original

    def test_raw_output_is_not_reemitted(self):
        self.events.insert(-1,{'Action':'output','Package':PACKAGE,'Output':'private token'})
        self.assertNotIn('private token',json.dumps(self.assess()))

    def test_other_explicit_skips_are_reported_not_erased(self):
        self.events[-1:-1]=[{'Action':'run','Package':PACKAGE,'Test':'OptionalStorage'},
                            {'Action':'skip','Package':PACKAGE,'Test':'OptionalStorage'}]
        self.assertEqual(self.assess()['skipped_test_events'],1)

    def test_malformed_duplicate_and_partial_lines_refuse(self):
        for suffix in ('not-json\n','{"Action":"output","Action":"pass"}\n','{}'):
            self.write()
            with (self.root/'go-test.jsonl').open('a') as f: f.write(suffix)
            with self.assertRaises((ValueError,TypeError)): m.assess(self.root,HEAD)

    def test_line_and_log_caps_refuse(self):
        self.write()
        with mock.patch.object(m,'MAX_LOG',1):
            with self.assertRaises(ValueError): m.assess(self.root,HEAD)
        with mock.patch.object(m,'MAX_LINE',8):
            with self.assertRaises(ValueError): m.assess(self.root,HEAD)

    def test_input_unchanged(self):
        self.write()
        before={p.name:p.read_bytes() for p in self.root.iterdir()}
        m.assess(self.root,HEAD)
        self.assertEqual(before,{p.name:p.read_bytes() for p in self.root.iterdir()})

    def test_missing_metadata_refuses(self):
        self.write();(self.root/'run.json').unlink()
        with self.assertRaises(ValueError): m.assess(self.root,HEAD)


    def test_cli_success_and_failure_keep_assessment(self):
        import subprocess
        import sys
        for exit_code,code in ((0,0),(1,1)):
            with self.subTest(exit_code=exit_code):
                self.meta['test_exit']=exit_code
                self.write()
                result=subprocess.run([sys.executable,str(Path(m.__file__)),str(self.root),
                                       '--expected-head',HEAD],capture_output=True,text=True)
                self.assertEqual(result.returncode,code)
                report=json.loads((self.root/'assessment.json').read_text())
                self.assertEqual(report['status'],'passed' if code==0 else 'failed')
                self.assertFalse(report['cleanup_authorized'])
                self.assertFalse(report['runtime_qualified'])

    def test_schema_boolean_and_duplicate_metadata_refuse(self):
        self.meta['schema']=True
        with self.assertRaises(ValueError): self.assess()
        self.meta['schema']=1
        self.write()
        path=self.root/'run.json'
        path.write_text('{"schema":1,'+path.read_text()[1:])
        with self.assertRaises(ValueError): m.assess(self.root,HEAD)

    def test_foreign_package_cannot_supply_missing_required_test(self):
        target=m.REQUIRED[0]
        for event in self.events:
            if event.get('Test')==target:
                event['Package']='foreign/package'
        with self.assertRaises(ValueError): self.assess()



    def test_wrapper_preserves_exit_failure_and_head_mismatch(self):
        import os
        import subprocess
        self.write()
        bindir=self.root/'bin';bindir.mkdir()
        programs={
            'git':"#!/bin/sh\nprintf '%s\\n' \"$STUB_HEAD\"\n",
            'go':"#!/bin/sh\nif [ \"$1\" = list ]; then printf '%s\\n' \"$STUB_PACKAGE\"; exit 0; fi\n/bin/cat \"$STUB_LOG\"\nexit \"$STUB_EXIT\"\n",
        }
        for name,body in programs.items():
            path=bindir/name;path.write_text(body);path.chmod(0o700)
        wrapper=Path(m.__file__).with_name('core-ci-evidence.sh')
        checkout=wrapper.parent.parent.parent
        for mode in ('pass','test_failure','stale_head','tee_failure'):
            with self.subTest(mode=mode):
                evidence=self.root/('evidence-'+mode)
                env=dict(os.environ,PATH=str(bindir)+os.pathsep+os.environ['PATH'],
                         STUB_HEAD=HEAD,STUB_PACKAGE=PACKAGE,STUB_LOG=str(self.root/'go-test.jsonl'),
                         STUB_EXIT='7' if mode=='test_failure' else '0',
                         GITHUB_SHA='b'*40 if mode=='stale_head' else HEAD)
                if mode=='tee_failure':
                    path=bindir/'tee';path.write_text('#!/bin/sh\n/bin/cat\nexit 9\n');path.chmod(0o700)
                result=subprocess.run(['bash',str(wrapper),str(evidence)],cwd=checkout,env=env,
                                      capture_output=True,text=True,timeout=15)
                self.assertEqual(result.returncode==0,mode=='pass')
                meta=json.loads((evidence/'run.json').read_text())
                self.assertEqual(meta['test_exit'],7 if mode=='test_failure' else 0)
                self.assertEqual(meta['tee_exit'],9 if mode=='tee_failure' else 0)
                report=json.loads((evidence/'assessment.json').read_text())
                self.assertEqual(report['status'],'passed' if mode=='pass' else 'failed')


if __name__=='__main__':
    unittest.main()
