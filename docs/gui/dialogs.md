# GUI 对话框规范

## 基本原则

表单对话框统一使用 `ResizableDialog` 组件（可拖拽、可调整大小）。

**禁止**使用手写的 `fixed inset-0` modal 实现。

## 组件导入

```tsx
import {
  ResizableDialog,
  ResizableDialogContent,
  ResizableDialogHeader,
  ResizableDialogBody,
  ResizableDialogFooter,
  ResizableDialogTitle,
} from '../ui/resizable-dialog'
import { Input, Checkbox, Button } from '../ui/dialog'
```

## 结构模板

```tsx
<ResizableDialog open={open} onOpenChange={(o) => !o && onClose()}>
  <ResizableDialogContent defaultWidth={450} defaultHeight={480} minWidth={350} minHeight={400}>
    <ResizableDialogHeader>
      <ResizableDialogTitle>标题</ResizableDialogTitle>
    </ResizableDialogHeader>
    <ResizableDialogBody>
      <form id="form-id" onSubmit={handleSubmit}>
        {/* 表单内容 */}
      </form>
    </ResizableDialogBody>
    <ResizableDialogFooter>
      <Button variant="outline" onClick={onClose}>Cancel</Button>
      <Button type="submit" form="form-id">Submit</Button>
    </ResizableDialogFooter>
  </ResizableDialogContent>
</ResizableDialog>
```

## 使用要点

1. `open` 和 `onOpenChange` 控制对话框的显示/隐藏
2. `defaultWidth` / `defaultHeight` 设置初始尺寸
3. `minWidth` / `minHeight` 设置最小尺寸限制
4. 表单使用 `id` 属性关联 Footer 中的提交按钮
5. Footer 中始终包含 Cancel 和 Submit 两个按钮
