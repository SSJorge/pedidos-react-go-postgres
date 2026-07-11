import React, { useCallback, useEffect, useState } from "react";

const API_URL =
  import.meta.env.VITE_API_URL ||
  "http://localhost:8080/api";
const TOKEN_KEY = "pedidos_access_token";
const USER_KEY = "pedidos_user";

const emptyLoginForm = {
  email: "",
  password: "",
};

const emptyForm = {
  customer_name: "",
  customer_email: "",
  product_name: "",
  quantity: 1,
  shipping_address: "",
  notes: "",
};

const statusLabels = {
  solicitado: "Solicitado",
  enviado: "Enviado",
  recibido: "Recibido",
};

function App() {
  const [form, setForm] = useState(emptyForm);
  const [orders, setOrders] = useState([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [token, setToken] = useState(() =>
  localStorage.getItem(TOKEN_KEY) || ""
);

const [user, setUser] = useState(() => {
  const storedUser = localStorage.getItem(USER_KEY);

  if (!storedUser) {
    return null;
  }

  try {
    return JSON.parse(storedUser);
  } catch {
    return null;
  }
});

const [loginForm, setLoginForm] = useState(emptyLoginForm);
const [loginLoading, setLoginLoading] = useState(false);

  const loadOrders = useCallback(async () => {
    try {
      setLoading(true);
      setError("");

      const response = await authenticatedFetch(
  `${API_URL}/orders`
);
      const data = await response.json();

      if (!response.ok) {
        throw new Error(data.error || "No se pudieron cargar los pedidos");
      }

      setOrders(data);
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setLoading(false);
    }
  }, [token]);

  useEffect(() => {
  if (token) {
    loadOrders();
  } else {
    setLoading(false);
  }
}, [token, loadOrders]);
  function logout() {
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(USER_KEY);

    setToken("");
    setUser(null);
    setOrders([]);
    setError("");
}
function updateLoginForm(event) {
  const { name, value } = event.target;

  setLoginForm((current) => ({
    ...current,
    [name]: value,
  }));
}

async function login(event) {
  event.preventDefault();

  try {
    setLoginLoading(true);
    setError("");

    const response = await fetch(`${API_URL}/auth/login`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(loginForm),
    });

    const data = await response.json();

    if (!response.ok) {
      throw new Error(
        data.error || "No se pudo iniciar sesión"
      );
    }

    localStorage.setItem(TOKEN_KEY, data.token);
    localStorage.setItem(USER_KEY, JSON.stringify(data.user));

    setToken(data.token);
    setUser(data.user);
    setLoginForm(emptyLoginForm);
  } catch (requestError) {
    setError(requestError.message);
  } finally {
    setLoginLoading(false);
  }
}
async function authenticatedFetch(url, options = {}) {
  const response = await fetch(url, {
    ...options,
    headers: {
      ...options.headers,
      Authorization: `Bearer ${token}`,
    },
  });

  if (response.status === 401) {
    logout();
  }

  return response;
}

  function updateForm(event) {
    const { name, value } = event.target;

    setForm((current) => ({
      ...current,
      [name]: name === "quantity" ? Number(value) : value,
    }));
  }

  async function createOrder(event) {
    event.preventDefault();

    try {
      setSaving(true);
      setError("");

      const response = await authenticatedFetch(
  `${API_URL}/orders`,
  {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(form),
      });

      const data = await response.json();

      if (!response.ok) {
        throw new Error(data.error || "No se pudo crear el pedido");
      }

      setOrders((current) => [data, ...current]);
      setForm(emptyForm);
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setSaving(false);
    }
  }

  async function changeStatus(orderId, status) {
    try {
      setError("");

      const response = await authenticatedFetch(
  `${API_URL}/orders/${orderId}/status`,
  {
        method: "PATCH",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ status }),
      });

      const data = await response.json();

      if (!response.ok) {
        throw new Error(data.error || "No se pudo actualizar el pedido");
      }

      setOrders((current) =>
        current.map((order) => (order.id === orderId ? data : order))
      );
    } catch (requestError) {
      setError(requestError.message);
    }
  }
  if (!token || !user) {
  return (
    <main className="login-page">
      <section className="login-card">
        <p className="eyebrow">Panel administrativo</p>
        <h1>Iniciar sesión</h1>
        <p>
          Ingresa con tu cuenta para gestionar los pedidos.
        </p>

        {error && <div className="error">{error}</div>}

        <form onSubmit={login} className="login-form">
          <label>
            Correo
            <input
              type="email"
              name="email"
              value={loginForm.email}
              onChange={updateLoginForm}
              autoComplete="email"
              required
            />
          </label>

          <label>
            Contraseña
            <input
              type="password"
              name="password"
              value={loginForm.password}
              onChange={updateLoginForm}
              autoComplete="current-password"
              required
            />
          </label>

          <button disabled={loginLoading}>
            {loginLoading
              ? "Ingresando..."
              : "Iniciar sesión"}
          </button>
        </form>
      </section>
    </main>
  );
}

  return (
    <main className="container">
      <header className="app-header">
  <div>
    <p className="eyebrow">MVP sin pagos</p>
    <h1>Gestión manual de pedidos</h1>
    <p>
      Registra solicitudes y cambia manualmente su estado a enviado o
      recibido.
    </p>
  </div>

  <div className="session-info">
    <span>
      Sesión iniciada como <strong>{user.name}</strong>
    </span>

    <button
      type="button"
      className="secondary"
      onClick={logout}
    >
      Cerrar sesión
    </button>
  </div>
</header>

      {error && <div className="error">{error}</div>}

      <section className="panel">
        <h2>Nueva solicitud</h2>

        <form onSubmit={createOrder} className="form-grid">
          <label>
            Nombre del cliente
            <input
              name="customer_name"
              value={form.customer_name}
              onChange={updateForm}
              required
            />
          </label>

          <label>
            Correo
            <input
              type="email"
              name="customer_email"
              value={form.customer_email}
              onChange={updateForm}
              required
            />
          </label>

          <label>
            Producto
            <input
              name="product_name"
              value={form.product_name}
              onChange={updateForm}
              required
            />
          </label>

          <label>
            Cantidad
            <input
              type="number"
              min="1"
              name="quantity"
              value={form.quantity}
              onChange={updateForm}
              required
            />
          </label>

          <label className="full-width">
            Dirección de envío
            <textarea
              name="shipping_address"
              value={form.shipping_address}
              onChange={updateForm}
              required
            />
          </label>

          <label className="full-width">
            Notas
            <textarea
              name="notes"
              value={form.notes}
              onChange={updateForm}
            />
          </label>

          <button disabled={saving}>
            {saving ? "Guardando..." : "Crear pedido"}
          </button>
        </form>
      </section>

      <section>
        <div className="section-heading">
          <h2>Pedidos</h2>
          <button className="secondary" onClick={loadOrders}>
            Actualizar
          </button>
        </div>

        {loading ? (
          <p>Cargando...</p>
        ) : orders.length === 0 ? (
          <p>No hay pedidos registrados.</p>
        ) : (
          <div className="orders">
            {orders.map((order) => (
              <article className="order-card" key={order.id}>
                <div className="order-header">
                  <div>
                    <span className="order-id">
  {order.public_code || `Pedido interno #${order.id}`}
</span>
                    <h3>{order.product_name}</h3>
                  </div>
                  <div className="order-identifiers">
  <span className="order-id">
    {order.public_code || "Sin código público"}
  </span>

  <small>ID interno: {order.id}</small>
</div>

                  <span className={`status status-${order.status}`}>
                    {statusLabels[order.status]}
                  </span>
                </div>

                <dl>
                  <div>
                    <dt>Cliente</dt>
                    <dd>{order.customer_name}</dd>
                  </div>
                  <div>
                    <dt>Correo</dt>
                    <dd>{order.customer_email}</dd>
                  </div>
                  <div>
                    <dt>Cantidad</dt>
                    <dd>{order.quantity}</dd>
                  </div>
                  <div>
                    <dt>Dirección</dt>
                    <dd>{order.shipping_address}</dd>
                  </div>
                </dl>

                {order.notes && <p className="notes">{order.notes}</p>}

                <div className="actions">
                  <button
                    className="secondary"
                    disabled={order.status === "solicitado"}
                    onClick={() => changeStatus(order.id, "solicitado")}
                  >
                    Volver a solicitado
                  </button>

                  <button
                    disabled={order.status === "enviado"}
                    onClick={() => changeStatus(order.id, "enviado")}
                  >
                    Marcar enviado
                  </button>

                  <button
                    disabled={order.status === "recibido"}
                    onClick={() => changeStatus(order.id, "recibido")}
                  >
                    Marcar recibido
                  </button>
                </div>
              </article>
            ))}
          </div>
        )}
      </section>
    </main>
  );
}

export default App;
