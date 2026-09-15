#!/usr/bin/env python3
"""Comprueba que la configuración del .env llega de verdad a los contenedores.

Existe por un fallo concreto. El CI escribía AUTH_RATE_LIMIT_PER_MINUTE en el
.env y la API arrancaba con su valor por defecto, porque la variable no estaba
declarada en el `environment:` de su servicio. Compose usa el .env para
interpolar el archivo, no para inyectar variables en los procesos: lo que no
se declara, no llega, y el servicio no se queja. El síntoma fueron veintitantas
pruebas de extremo a extremo fallando con 429, que parecían un problema del
limitador de tasa y eran un problema de configuración.

Se ejecuta ANTES de levantar nada, sobre la salida de `docker compose config`,
así que no necesita demonio y falla en segundos en vez de después de construir
las imágenes y correr la suite entera.

Uso:
    docker compose config > resuelto.yml
    python3 .github/scripts/verificar-entorno.py resuelto.yml \\
        api:AUTH_RATE_LIMIT_PER_MINUTE=300 api:ADMIN_EMAIL=admin@ci.local
"""

import sys

import yaml


def main(argv: list[str]) -> int:
    if len(argv) < 3:
        print(__doc__, file=sys.stderr)
        return 2

    ruta, esperado = argv[1], argv[2:]
    with open(ruta, encoding="utf-8") as f:
        servicios = yaml.safe_load(f).get("services", {})

    fallos = []
    for expresion in esperado:
        try:
            servicio, resto = expresion.split(":", 1)
            clave, valor = resto.split("=", 1)
        except ValueError:
            fallos.append(f"{expresion!r} no tiene la forma servicio:CLAVE=valor")
            continue

        if servicio not in servicios:
            fallos.append(f"no existe el servicio {servicio!r}")
            continue

        # Compose normaliza `environment` a diccionario en su salida resuelta,
        # pero acepta lista en la entrada; se contemplan las dos.
        entorno = servicios[servicio].get("environment") or {}
        if isinstance(entorno, list):
            entorno = dict(
                par.split("=", 1) if "=" in par else (par, "") for par in entorno
            )

        actual = entorno.get(clave)
        if actual is None:
            fallos.append(
                f"{servicio}: {clave} no llega al contenedor. "
                f"Ponerlo en el .env no basta: hay que declararlo en el "
                f"`environment:` del servicio en docker-compose.yml."
            )
        elif str(actual) != valor:
            fallos.append(f"{servicio}: {clave} es {actual!r} y se esperaba {valor!r}")

    if fallos:
        print("La configuración no llega a los contenedores:", file=sys.stderr)
        for f in fallos:
            print(f"  - {f}", file=sys.stderr)
        return 1

    print(f"Configuración verificada: {len(esperado)} valor(es) llegan al contenedor.")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
