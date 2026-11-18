"""Read-only assessment of CI test discovery/execution, never safety clearance."""
import argparse
import json
from pathlib import Path
import re

REQUIRED = (
    'TestRecoveryContractProductionLifecycleAndPersistenceFailures',
    'TestRecoveryContractFailedPinInspectionReflectsActualPrefix',
    'TestRecoveryContractSnapshotMutationAtVerifiedReopenBoundary',
    'TestRecoveryContractAtomicReplacementHasNoTornReaderSuccess',
    'TestRecoveryContractNestedSchemaRejectsEveryRequiredFieldMutation',
    'TestRecoveryContractDiagnosticRelationshipsRejectContradictions',
    'FuzzRecoveryContractStrictManifest',
)
FUZZ = REQUIRED[-1]
COMMAND = ['go','test','-json','-race','-count=1','-timeout=120s','./...']
MAX_LOG = 64 << 20
MAX_LINE = 1 << 20


def require(value):
    if not value:
        raise ValueError('invalid_or_incomplete_evidence')


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result)
        result[key] = value
    return result


def decode(text):
    return json.loads(text, object_pairs_hook=unique_object,
                      parse_constant=lambda _: (_ for _ in ()).throw(ValueError()))


def assess(directory, expected_head):
    require(re.fullmatch(r'[0-9a-f]{40}', expected_head))
    meta_path = directory / 'run.json'
    require(meta_path.is_file() and not meta_path.is_symlink() and meta_path.stat().st_size <= 16384)
    meta = decode(meta_path.read_text(encoding='utf-8'))
    require(isinstance(meta, dict) and type(meta.get('schema')) is int and meta.get('schema') == 1)
    require(meta.get('head') == expected_head and meta.get('command') == COMMAND)
    require(type(meta.get('test_exit')) is int and meta['test_exit'] == 0)
    require(type(meta.get('tee_exit')) is int and meta['tee_exit'] == 0)
    require(meta.get('runtime_qualified') is False and meta.get('cleanup_authorized') is False)
    package = meta.get('root_package')
    require(isinstance(package,str) and bool(package) and len(package) <= 512)
    path = directory / 'go-test.jsonl'
    require(path.is_file() and not path.is_symlink() and 0 < path.stat().st_size <= MAX_LOG)
    tests, packages = {}, {}
    skipped = 0
    with path.open('rb') as stream:
        for raw in iter(lambda: stream.readline(MAX_LINE + 1), b''):
            require(len(raw) <= MAX_LINE and raw.endswith(b'\n'))
            event = decode(raw.decode('utf-8','strict'))
            require(isinstance(event,dict))
            action, pkg, name = event.get('Action'), event.get('Package'), event.get('Test')
            require(action in ('start','run','pause','cont','pass','bench','fail','output','skip','build-output','build-fail'))
            require(action not in ('fail','build-fail'))
            require(isinstance(pkg,str) and bool(pkg))
            if name is not None:
                require(isinstance(name,str) and bool(name))
                if action == 'skip':
                    skipped += 1
                if action in ('run','pass','skip'):
                    key = (pkg,name)
                    previous = tests.get(key)
                    if action == 'run':
                        require(previous is None)
                    else:
                        require(previous == 'run')
                    tests[key] = action
            elif action in ('start','pass','skip'):
                previous = packages.get(pkg)
                require((action == 'start' and previous is None) or
                        (action in ('pass','skip') and previous == 'start'))
                packages[pkg] = action
    require(packages.get(package) == 'pass')
    require(all(status in ('pass','skip') for status in packages.values()))
    require(all(status in ('pass','skip') for status in tests.values()))
    expected = list(REQUIRED) + [f'{FUZZ}/seed#{i}' for i in range(10)]
    for name in expected:
        require(tests.get((package,name)) == 'pass')
    return {'schema':1,'head':expected_head,'status':'passed',
            'required_top_level_tests':len(REQUIRED),'required_fuzz_seeds':10,
            'skipped_test_events':skipped,'package_count':len(packages),
            'runtime_qualified':False,'cleanup_authorized':False,
            'scope':'execution evidence only; not signed provenance or whole-audit clearance'}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory',type=Path)
    parser.add_argument('--expected-head',required=True)
    args = parser.parse_args()
    result = {'schema':1,'status':'failed','reason':'invalid_or_incomplete_evidence',
              'runtime_qualified':False,'cleanup_authorized':False}
    try:
        result = assess(args.directory,args.expected_head)
    except (OSError,ValueError,TypeError,KeyError,RecursionError,OverflowError):
        pass
    args.directory.mkdir(parents=True,exist_ok=True)
    (args.directory/'assessment.json').write_text(json.dumps(result,indent=2)+'\n',encoding='utf-8')
    print(json.dumps(result,sort_keys=True))
    return 0 if result['status'] == 'passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())
