# Verificador de citas

Aplicacion local en Go para consultar citas proximas de Calendly dentro de una ventana configurable y administrarla desde una interfaz web en HTML, CSS y JavaScript puros.

## Ejecutar

1. Completa tu `.env` con el token de Calendly.
2. Inicia el servidor:

```bash
make serve
```

3. Abre `http://localhost:8080`.

## Makefile

- `make serve`: levanta el servidor en desarrollo.
- `make build`: genera el binario en `./bin/verificador-citas`.
- `make test`: corre los tests.
- `make fmt`: formatea el codigo Go.
- `make check`: ejecuta formato, tests y build.
- `make clean`: elimina binarios generados sin tocar `data/`.

## Variables de entorno

- `CALENDLY_API_BASE_URL`
- `CALENDLY_API_TOKEN`
- `CALENDLY_ORGANIZATION_URI` (opcional)
- `CALENDLY_EVENT_TYPE_URI` (opcional, se usa como filtro local)
- `CALENDLY_VALIDATION_PAGE_SIZE`
- `SERVER_ADDR` (opcional, default `:8080`)
- `APP_DATA_DIR` (opcional, default `data`)

## Notas

- La identidad del invitado se resuelve despues de listar las citas encontradas.
- La configuracion editable se guarda en `data/config.json`.
- El historial de ejecuciones se guarda en `data/history.json`.
