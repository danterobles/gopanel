# Resultado Go local — etapa 8

Fecha: 2026-10-22. Protocolo: [benchmark](protocolo-benchmark.md); decisión SQL [ADR-0010](adr/0010-orden-reciente-medido.md). Sin contraparte Laravel/Filament disponible: no hay conclusión comparativa.

Apple M2 Max, 12 CPU lógicas, 32 GiB RAM, macOS ARM64, Go 1.26.10, MySQL 8.0.40; assets compilados con Node 22.23.2. MySQL y generador de carga en el mismo host. Binario sin race; pool máximo 10 conexiones. Dos empresas, 10 000 clientes y 10 000 productos por empresa; 50 identidades Viewer con sesiones independientes, 25 por tenant. HTML completo, página 1 de 25 filas, created_desc; 100 requests de warmup y 20 s configurados por recurso. Fixtures lógicamente equivalentes recreados, con UUID aleatorios.

Un ensayo inicial concurrente con otras pruebas quedó excluido. Baseline y corrida posterior se ejecutaron aisladas, con idéntico protocolo. EXPLAIN ANALYZE mostró Sort/10 000 filas con CASE; Recent usa el índice existente en reversa y obtiene 25 sin Sort. No se añadió caché ni migración.

| Recurso | p95 baseline ms | p50 posterior ms | p95 posterior ms | RPS posterior | Muestras | Errores |
|---|---|---|---|---|---|---|
| customers | 614.40 | 138.02 | 286.38 | 329.4 | 6632 | 0 |
| products | 622.58 | 141.40 | 287.91 | 322.3 | 6491 | 0 |

Pico de RSS del servidor muestreado: 148.4 MiB; heap 113.9 MiB; memoria Sys de Go 144.3 MiB. Son máximos observados cada 500 ms, no un perfil exhaustivo. RPS incluye el tiempo de finalizar solicitudes en curso al terminar la ventana.

La corrida posterior cumple p95 < 300 ms en ambos recursos con margen pequeño. Es una muestra por recurso y versión, sin garantía de repetibilidad o producción. No cubre búsquedas, otros órdenes, páginas profundas, escrituras, login o SSE. No debe compararse con resultados de otro stack sin repetir el mismo protocolo en ambos.

Artefactos privados: .local/benchmark-go.json, benchmark-go-baseline.json, benchmark-go-concurrent-trial.json y benchmark-query-plans*.json. Los planes guardados anonimizaron el tenant de prueba; no se publican credenciales ni dumps.

## LOC de recursos

Conteo reproducible con `python3 scripts/count-loc.py`: líneas físicas no vacías, separado el código generado. No hay DSL declarativa de recursos, por lo que no se puede equiparar el código de dominio con la configuración de un recurso Filament ni inferir horas humanas.

| Recurso | Go escrito (sin pruebas) | Templ | SQL | Go generado (vistas + SQL) |
|---|---|---|---|---|
| customers | 607 | 185 | 31 | 1178 |
| products | 657 | 170 | 33 | 1256 |

El conteo no atribuye componentes compartidos a un recurso y no mezcla pruebas con implementación. El esfuerzo del segundo CRUD y extracción permanece en [registro de esfuerzo](registro-de-esfuerzo.md). Comparación de esfuerzo/LOC/rendimiento con Laravel/Filament pendiente.
