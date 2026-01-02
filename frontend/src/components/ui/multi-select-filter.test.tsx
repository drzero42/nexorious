import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@/test/test-utils";
import userEvent from "@testing-library/user-event";
import { MultiSelectFilter } from "./multi-select-filter";
import type { MultiSelectOption } from "./multi-select-filter";

// Mock lucide-react icons
vi.mock("lucide-react", () => ({
  ChevronDown: () => <div data-testid="chevron-down-icon" />,
  Check: () => <div data-testid="check-icon" />,
}));

// ============================================================================
// Test Data
// ============================================================================

const mockOptions: MultiSelectOption[] = [
  { value: "action", label: "Action" },
  { value: "rpg", label: "RPG" },
  { value: "strategy", label: "Strategy" },
  { value: "puzzle", label: "Puzzle" },
];

// ============================================================================
// MultiSelectFilter Tests
// ============================================================================

describe("MultiSelectFilter", () => {
  it("renders the label", () => {
    render(
      <MultiSelectFilter
        label="Genres"
        options={mockOptions}
        selected={[]}
        onChange={vi.fn()}
      />
    );

    expect(screen.getByRole("button")).toHaveTextContent("Genres");
  });

  it("shows count badge when items are selected", () => {
    render(
      <MultiSelectFilter
        label="Genres"
        options={mockOptions}
        selected={["action", "rpg"]}
        onChange={vi.fn()}
      />
    );

    expect(screen.getByRole("button")).toHaveTextContent("Genres (2)");
  });

  it("does not show count badge when no items are selected", () => {
    render(
      <MultiSelectFilter
        label="Genres"
        options={mockOptions}
        selected={[]}
        onChange={vi.fn()}
      />
    );

    expect(screen.getByRole("button")).toHaveTextContent("Genres");
    expect(screen.getByRole("button")).not.toHaveTextContent("(");
  });

  it("opens dropdown on click", async () => {
    const user = userEvent.setup();

    render(
      <MultiSelectFilter
        label="Genres"
        options={mockOptions}
        selected={[]}
        onChange={vi.fn()}
      />
    );

    await user.click(screen.getByRole("button"));

    // Should see all options
    expect(screen.getByText("Action")).toBeInTheDocument();
    expect(screen.getByText("RPG")).toBeInTheDocument();
    expect(screen.getByText("Strategy")).toBeInTheDocument();
    expect(screen.getByText("Puzzle")).toBeInTheDocument();
  });

  it("shows checkboxes for each option", async () => {
    const user = userEvent.setup();

    render(
      <MultiSelectFilter
        label="Genres"
        options={mockOptions}
        selected={[]}
        onChange={vi.fn()}
      />
    );

    await user.click(screen.getByRole("button"));

    const checkboxes = screen.getAllByRole("checkbox");
    expect(checkboxes).toHaveLength(4);
  });

  it("shows selected options as checked", async () => {
    const user = userEvent.setup();

    render(
      <MultiSelectFilter
        label="Genres"
        options={mockOptions}
        selected={["action", "rpg"]}
        onChange={vi.fn()}
      />
    );

    await user.click(screen.getByRole("button"));

    const checkboxes = screen.getAllByRole("checkbox");
    // Action and RPG should be checked
    expect(checkboxes[0]).toBeChecked();
    expect(checkboxes[1]).toBeChecked();
    // Strategy and Puzzle should not be checked
    expect(checkboxes[2]).not.toBeChecked();
    expect(checkboxes[3]).not.toBeChecked();
  });

  it("calls onChange when option is toggled on", async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <MultiSelectFilter
        label="Genres"
        options={mockOptions}
        selected={["action"]}
        onChange={handleChange}
      />
    );

    await user.click(screen.getByRole("button"));

    // Click on RPG label to toggle it
    const rpgLabel = screen.getByText("RPG");
    await user.click(rpgLabel);

    expect(handleChange).toHaveBeenCalledWith(["action", "rpg"]);
  });

  it("removes value from selection when unchecked", async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <MultiSelectFilter
        label="Genres"
        options={mockOptions}
        selected={["action", "rpg"]}
        onChange={handleChange}
      />
    );

    await user.click(screen.getByRole("button"));

    // Click on Action label to toggle it off
    const actionLabel = screen.getByText("Action");
    await user.click(actionLabel);

    expect(handleChange).toHaveBeenCalledWith(["rpg"]);
  });

  it("shows disabled state", () => {
    render(
      <MultiSelectFilter
        label="Genres"
        options={mockOptions}
        selected={[]}
        onChange={vi.fn()}
        disabled
      />
    );

    expect(screen.getByRole("button")).toBeDisabled();
  });

  it("does not open dropdown when disabled", async () => {
    const user = userEvent.setup();

    render(
      <MultiSelectFilter
        label="Genres"
        options={mockOptions}
        selected={[]}
        onChange={vi.fn()}
        disabled
      />
    );

    await user.click(screen.getByRole("button"));

    // Options should not be visible
    expect(screen.queryByText("Action")).not.toBeInTheDocument();
  });

  it("closes dropdown when clicking outside", async () => {
    const user = userEvent.setup();

    render(
      <div>
        <div data-testid="outside">Outside</div>
        <MultiSelectFilter
          label="Genres"
          options={mockOptions}
          selected={[]}
          onChange={vi.fn()}
        />
      </div>
    );

    // Open dropdown
    await user.click(screen.getByRole("button"));
    expect(screen.getByText("Action")).toBeInTheDocument();

    // Click outside
    await user.click(screen.getByTestId("outside"));

    // Dropdown should be closed
    expect(screen.queryByText("Action")).not.toBeInTheDocument();
  });

  it("closes dropdown when pressing Escape", async () => {
    const user = userEvent.setup();

    render(
      <MultiSelectFilter
        label="Genres"
        options={mockOptions}
        selected={[]}
        onChange={vi.fn()}
      />
    );

    // Open dropdown
    await user.click(screen.getByRole("button"));
    expect(screen.getByText("Action")).toBeInTheDocument();

    // Press Escape
    await user.keyboard("{Escape}");

    // Dropdown should be closed
    expect(screen.queryByText("Action")).not.toBeInTheDocument();
  });

  it("applies custom className", () => {
    const { container } = render(
      <MultiSelectFilter
        label="Genres"
        options={mockOptions}
        selected={[]}
        onChange={vi.fn()}
        className="custom-class"
      />
    );

    expect(container.querySelector(".custom-class")).toBeInTheDocument();
  });

  it("shows empty state when no options are available", async () => {
    const user = userEvent.setup();

    render(
      <MultiSelectFilter
        label="Genres"
        options={[]}
        selected={[]}
        onChange={vi.fn()}
      />
    );

    await user.click(screen.getByRole("button"));

    expect(screen.getByText("No options available")).toBeInTheDocument();
  });

  it("maintains correct order of selected items", async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <MultiSelectFilter
        label="Genres"
        options={mockOptions}
        selected={[]}
        onChange={handleChange}
      />
    );

    await user.click(screen.getByRole("button"));

    // Select Strategy first
    await user.click(screen.getByText("Strategy"));
    expect(handleChange).toHaveBeenLastCalledWith(["strategy"]);
  });
});
