import { useState, useMemo } from "react";
import { Input, Popover } from "antd";
import { Search } from "lucide-react";
import type {
  CustomDropdownProps,
  CustomDropdownItem,
  DropdownPanelProps,
  DropdownItemRowProps,
} from "./types";
import styles from "./CustomDropdown.module.css";

function filterItems(
  items: CustomDropdownItem[],
  query: string,
): CustomDropdownItem[] {
  if (!query) return items;
  const lower = query.toLowerCase();
  return items.reduce<CustomDropdownItem[]>((acc, item) => {
    if (item.label.toLowerCase().includes(lower)) {
      acc.push(item);
    } else if (item.children) {
      const filtered = filterItems(item.children, query);
      if (filtered.length) acc.push({ ...item, children: filtered });
    }
    return acc;
  }, []);
}

function DropdownPanel({
  items,
  searchable,
  minWidth,
  onItemClick,
}: DropdownPanelProps) {
  const [search, setSearch] = useState("");

  const filtered = useMemo(
    () => filterItems(items, search),
    [items, search],
  );

  return (
    <div style={{ minWidth }}>
      {searchable && items.length > 0 && (
        <Input
          className={styles.search_input}
          placeholder="Search..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          onClick={(e) => e.stopPropagation()}
          autoFocus
          prefix={
            <Search className="w-4 h-4 text-gray-400" />
          }
        />
      )}
      <div className={styles.items_container}>
        {filtered.map((item) => (
          <div key={item.key}>
            {item.dividerBefore && (
              <div className="my-1 -mx-1 border-t border_8" />
            )}
            <DropdownItemRow
              item={item}
              searchable={searchable}
              minWidth={minWidth}
              onItemClick={onItemClick}
            />
          </div>
        ))}
        {searchable && filtered.length === 0 && (
          <div className="px-3 py-2 text-xs text_color opacity-50">
            No results
          </div>
        )}
      </div>
    </div>
  );
}

function DropdownItemRow({
  item,
  searchable,
  minWidth,
  onItemClick,
}: DropdownItemRowProps) {
  const hasSuffix = !!(item.suffix || item.children);

  const itemEl = (
    <div
      className={`dropdown_item${item.disabled ? " dropdown_item_disabled" : ""}`}
      onClick={() => onItemClick(item)}
    >
      {item.icon}
      <span className={hasSuffix ? "flex-1" : undefined}>
        {item.label}
      </span>
      {hasSuffix && item.suffix}
    </div>
  );

  if (item.children) {
    return (
      <Popover
        content={
          <DropdownPanel
            items={item.children}
            searchable={item.childSearchable ?? searchable}
            minWidth={minWidth}
            onItemClick={onItemClick}
          />
        }
        placement="rightTop"
        trigger="hover"
        arrow={false}
        overlayClassName={styles.popover_overlay}
      >
        {itemEl}
      </Popover>
    );
  }

  return itemEl;
}

export default function CustomDropdown({
  items,
  trigger,
  placement = "topLeft",
  open: controlledOpen,
  onOpenChange,
  minWidth = 200,
  searchable = false,
  closeOnSelect = true,
}: CustomDropdownProps) {
  const [internalOpen, setInternalOpen] = useState(false);

  const isControlled = controlledOpen !== undefined;
  const open = isControlled ? controlledOpen : internalOpen;

  const setOpen = (value: boolean) => {
    if (!isControlled) setInternalOpen(value);
    onOpenChange?.(value);
  };

  const handleItemClick = (item: CustomDropdownItem) => {
    if (item.children || item.disabled) return;
    if (closeOnSelect) setOpen(false);
    item.onClick?.();
  };

  return (
    <Popover
      content={
        <DropdownPanel
          items={items}
          searchable={searchable}
          minWidth={minWidth}
          onItemClick={handleItemClick}
        />
      }
      trigger="click"
      placement={placement}
      arrow={false}
      open={open}
      onOpenChange={setOpen}
      overlayClassName={styles.popover_overlay}
    >
      {trigger}
    </Popover>
  );
}
