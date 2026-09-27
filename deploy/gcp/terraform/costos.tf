# Presupuesto con alertas. Opcional: exige administrar la cuenta de
# facturación (roles/billing.costsManager o superior) y la API
# billingbudgets.googleapis.com.

resource "google_billing_budget" "mensual" {
  count           = var.cuenta_facturacion != "" ? 1 : 0
  billing_account = var.cuenta_facturacion
  display_name    = "${var.prefijo} - entrega 2"

  budget_filter {
    projects = ["projects/${data.google_project.actual.number}"]
    # Con créditos educativos el gasto neto es 0 y un presupuesto que
    # descuenta créditos no avisaría nunca. Se mide el costo bruto, que es
    # lo que se está consumiendo del crédito.
    credit_types_treatment = "EXCLUDE_ALL_CREDITS"
  }

  amount {
    specified_amount {
      currency_code = var.presupuesto_moneda
      units         = tostring(var.presupuesto_monto)
    }
  }

  threshold_rules {
    threshold_percent = 0.5
  }
  threshold_rules {
    threshold_percent = 0.9
  }
  threshold_rules {
    threshold_percent = 1.0
  }
  # Aviso por proyección: llega antes de gastar, no después.
  threshold_rules {
    threshold_percent = 1.0
    spend_basis       = "FORECASTED_SPEND"
  }

  # Sin all_updates_rule los correos van a los administradores y usuarios de
  # la cuenta de facturación, que es lo que se quiere aquí.
}
