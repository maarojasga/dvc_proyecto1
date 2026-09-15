#!/usr/bin/env python3
"""Lee el resumen de k6 y dice en cinco líneas si la corrida vale.

Existe por lo que cuenta load/README.md en «Por qué setup() valida el escenario
entero»: hubo un periodo en que esta prueba informaba 0 % de error y 100 % de
comprobaciones con la mitad del tráfico sin ejecutar. El resumen de k6 tiene
cientos de líneas y ese informe parecía verde.

Así que además de los umbrales comprueba lo que aquel informe no comprobaba:
que los cuatro escenarios hicieron tráfico de verdad. Un escenario que no llega
a hacer una sola petición HTTP no aparece en el resumen, y su ausencia es lo que
hay que detectar.

Sale con código 1 si algo falla, para poder colgarlo de un paso de CI.

    python3 load/leer-resumen.py load/salida/resumen-etapa1.json
"""

import json
import sys

ESCENARIOS = ("catalogo", "consumo", "quiz", "login")


def main(ruta: str) -> int:
    with open(ruta, encoding="utf-8") as f:
        metricas = json.load(f)["metrics"]

    problemas = []

    print(f"{'escenario':<12}{'p95 (ms)':>10}")
    for nombre in ESCENARIOS:
        metrica = metricas.get(f"http_req_duration{{escenario:{nombre}}}")
        if metrica is None:
            # Sin una sola petición HTTP no hay métrica. No es «rápido»: es que
            # no corrió.
            print(f"{nombre:<12}{'SIN TRÁFICO':>10}")
            problemas.append(f"el escenario {nombre} no hizo ninguna petición")
            continue
        print(f"{nombre:<12}{metrica['values']['p(95)']:>10.1f}")

    fallos = metricas["http_req_failed"]["values"]
    total = fallos["passes"] + fallos["fails"]
    comprobaciones = metricas["checks"]["values"]
    print(f"\npeticiones:     {total}")
    print(f"errores:        {fallos['rate'] * 100:.3f} %")
    print(f"comprobaciones: {comprobaciones['passes']} pasadas, {comprobaciones['fails']} fallidas")

    if total == 0:
        problemas.append("la corrida no hizo ninguna petición")

    rotos = [
        f"{nombre} :: {expresion}"
        for nombre, metrica in metricas.items()
        if isinstance(metrica, dict)
        for expresion, estado in (metrica.get("thresholds") or {}).items()
        if not estado.get("ok", True)
    ]
    print("umbrales rotos:", ", ".join(rotos) if rotos else "ninguno")
    problemas.extend(f"umbral incumplido: {r}" for r in rotos)

    if problemas:
        print("\nLa corrida NO vale:")
        for p in problemas:
            print(f"  - {p}")
        return 1

    print("\nLa corrida vale: los cuatro escenarios hicieron tráfico y ningún umbral se rompió.")
    return 0


if __name__ == "__main__":
    if len(sys.argv) != 2:
        print(__doc__)
        raise SystemExit(2)
    raise SystemExit(main(sys.argv[1]))
