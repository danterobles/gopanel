"""Physical nonblank source lines, separated from generated code; no productivity inference."""
import json
from pathlib import Path

def lines(path):
    return sum(bool(line.strip()) for line in path.read_text().splitlines())

result = {'resources': {}, 'configuration': {}, 'resource_configuration_dsl': None,
          'note': 'No declarative resource DSL exists. Do not equate domain code or generated Go with Filament configuration or human effort.'}
for resource in ('customers', 'products'):
    module = Path('internal/modules') / resource
    result['resources'][resource] = {
        'handwritten_go_nonblank': sum(lines(p) for p in module.rglob('*.go') if not p.name.endswith('_test.go')),
        'templ_nonblank': lines(Path(f'internal/ui/pages/{resource}.templ')),
        'sql_nonblank': lines(Path(f'internal/db/queries/{resource}.sql')),
        'generated_go_nonblank': lines(Path(f'internal/ui/pages/{resource}_templ.go')) + lines(Path(f'internal/db/generated/{resource}.sql.go')),
    }
for path in ('Makefile', 'sqlc.yaml', 'tools/versions.mk'):
    result['configuration'][path] = lines(Path(path))
print(json.dumps(result, indent=2))
