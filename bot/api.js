// Client for the api's /internal/sharecodes endpoints (worker token).

export class API {
  /**
   * @param {string} base  e.g. https://csdc.example.com
   * @param {string} token CSDC_WORKER_TOKEN
   */
  constructor(base, token) {
    this.base = base.replace(/\/+$/, "");
    this.token = token;
  }

  /** @returns {Promise<{id: number, code: string} | null>} the next code, or null */
  async claim() {
    const res = await this.#post("/internal/sharecodes/claim", {});
    if (res.status === 204) return null;
    return res.json();
  }

  async resolve(id, url, played) {
    await this.#post(`/internal/sharecodes/${id}/resolve`, { demo_url: url, played });
  }

  /**
   * @param {number} id
   * @param {string} reason
   * @param {{expired?: boolean, permanent?: boolean}} [kind]
   */
  async fail(id, reason, kind = {}) {
    await this.#post(`/internal/sharecodes/${id}/fail`, { reason, ...kind });
  }

  async #post(path, body) {
    const res = await fetch(this.base + path, {
      method: "POST",
      headers: { Authorization: `Bearer ${this.token}`, "Content-Type": "application/json" },
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(30_000),
    });
    if (!res.ok) throw new Error(`api ${path}: ${res.status} ${(await res.text()).trim()}`);
    return res;
  }
}
