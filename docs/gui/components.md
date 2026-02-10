# GUI 组件使用规范

## 基本原则

优先使用项目已有的 UI 组件，而非原生 HTML 元素。这些组件已包含完整的交互反馈（hover、focus、disabled 状态）和一致的视觉风格。

## 组件列表

### Button

```tsx
import { Button } from '../ui/dialog'
```

支持 `variant` 属性：
- `default` - 默认样式（主要操作）
- `outline` - 边框样式（次要操作）
- `destructive` - 危险操作样式（删除等）

### Input

```tsx
import { Input } from '../ui/dialog'
```

带 label 的输入框组件。

### Checkbox

```tsx
import { Checkbox } from '../ui/dialog'
```

### Select

```tsx
import { Select } from '../ui/dialog'
```

### Tabs

```tsx
import { Tabs, TabsList, TabsTrigger, TabsContent } from '../ui/tabs'
```

标签页组件，用于内容分组切换。

### ScrollArea

```tsx
import { ScrollArea } from '../ui/scroll-area'
```

自定义滚动区域组件，提供统一的滚动条样式。
