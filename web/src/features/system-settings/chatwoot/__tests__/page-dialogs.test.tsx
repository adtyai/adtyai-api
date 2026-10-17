/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ROLE } from "@/lib/roles";
import { useAuthStore } from "@/stores/auth-store";

import { ChatwootSettings } from "../index";
import type { ChatwootConfigView } from "../types";

const sampleConfig: ChatwootConfigView = {
  script: "<script>old</script>",
  website_token: "tok",
  base_url: "https://chat.example.com",
  hmac_secret_set: true,
  hmac_secret_fingerprint: "abcd1234",
  backup_contact_type: "email",
  backup_contact: "ops@example.com",
  backup_contact_note: "",
  warnings: [],
};

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    children,
    to,
    ...rest
  }: {
    children: React.ReactNode;
    to: string;
    className?: string;
  }) => (
    <a href={to} {...rest}>
      {children}
    </a>
  ),
}));

const { getChatwootConfig, putChatwootConfig } = vi.hoisted(() => ({
  getChatwootConfig: vi.fn(),
  putChatwootConfig: vi.fn(),
}));

vi.mock("../api", () => ({
  getChatwootConfig,
  putChatwootConfig,
}));

function renderPage() {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return render(
    <QueryClientProvider client={client}>
      <ChatwootSettings />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: "root", role: ROLE.SUPER_ADMIN });
  getChatwootConfig.mockResolvedValue(sampleConfig);
  putChatwootConfig.mockResolvedValue({
    success: true,
    data: sampleConfig,
  });
});

afterEach(() => {
  vi.clearAllMocks();
  useAuthStore.getState().auth.reset();
});

describe("ChatwootSettings ConfirmDialog", () => {
  it("mounts ConfirmDialog (alertdialog) when Save changes script", async () => {
    const user = userEvent.setup();
    renderPage();

    const script = await screen.findByLabelText("Chatwoot script");
    await user.clear(script);
    await user.type(script, "<script>new</script>");

    await user.click(screen.getByRole("button", { name: "Save Changes" }));

    // ConfirmDialog is AlertDialog → role=alertdialog (ARIA); still a visible dialog.
    expect(await screen.findByRole("alertdialog")).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Confirm Chatwoot sensitive save" }),
    ).toBeInTheDocument();
  });
});
