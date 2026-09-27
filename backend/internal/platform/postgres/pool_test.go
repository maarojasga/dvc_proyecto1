package postgres

import "testing"

// El pool de la API y el del worker se suman contra el max_connections de la
// base administrada. DB_MAX_CONNS tiene que llegar tal cual al pool, y un
// valor no positivo no puede dejar un pool vacío, que colgaría la primera
// consulta en lugar de fallar.
func TestTamanoDelPool(t *testing.T) {
	const url = "postgres://mooc:x@10.30.0.3:5432/mooc?sslmode=require"
	casos := []struct {
		pedido int
		quiero int32
	}{
		{pedido: 12, quiero: 12},
		{pedido: 0, quiero: conexionesPorDefecto},
		{pedido: -3, quiero: conexionesPorDefecto},
		{pedido: 1 << 40, quiero: conexionesMaximas},
	}
	for _, c := range casos {
		cfg, err := configDelPool(url, c.pedido)
		if err != nil {
			t.Fatalf("configDelPool(%d): %v", c.pedido, err)
		}
		if cfg.MaxConns != c.quiero {
			t.Errorf("DB_MAX_CONNS=%d: pool de %d, esperaba %d", c.pedido, cfg.MaxConns, c.quiero)
		}
	}
}

// sslmode=require es lo que usan los compose de GCP contra Cloud SQL por IP
// privada: cifra sin exigir el certificado de la CA del servidor.
func TestLaURLConservaElModoTLS(t *testing.T) {
	cfg, err := configDelPool("postgres://mooc:x@10.30.0.3:5432/mooc?sslmode=require", 20)
	if err != nil {
		t.Fatalf("configDelPool: %v", err)
	}
	if cfg.ConnConfig.TLSConfig == nil {
		t.Fatal("sslmode=require debería producir una conexión con TLS")
	}
}
